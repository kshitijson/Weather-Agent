# Deploying with Jenkins and the Weather Agent on separate EC2 instances

This guide splits the setup from [DEPLOYMENT.md](DEPLOYMENT.md) across two servers:

- **EC2 #1 (Jenkins):** runs only the Jenkins UI and schedules builds. No Docker builds happen here.
- **EC2 #2 (App):** registered in Jenkins as a **build agent**. When you click Build Now, Jenkins connects to this instance over SSH, and the checkout, `docker build` and `docker run` all happen **on EC2 #2**. The app container ends up running there.

```
Your Mac ──browser :8080──▶ EC2 #1: Jenkins controller (container)
                                   │
                                   │ SSH (port 22, private IP)
                                   ▼
Users ────HTTP :80────────▶ EC2 #2: Jenkins agent
                                   ├─ git clone Weather-Agent
                                   ├─ docker build  → weather-agent image
                                   └─ docker run    → weather-agent container (:80 → :8080)
```

### Why split it
- A Jenkins crash or restart doesn't take the app down, and an app build can't run Jenkins out of memory.
- Jenkins no longer needs access to Docker at all: no socket mount, no socat.
- It's how real setups work: one Jenkins controller, many build/deploy machines.

### Cost
Free Tier hours are a **monthly total across all instances**, not per instance:

- **Older accounts** (750 hrs/month): two instances running all month use about 1,460 hours, so about half a month is billed. Stop both when you're not using them.
- **Newer accounts** (credits): credits drain twice as fast.
- Both instances' disks count toward the 30 GB free storage. Use **15 GB each**.

---

## Step 1: Create the security groups

Create both groups first, because the app's group refers to the Jenkins group.

EC2 → **Security Groups** → **Create security group**.

### `jenkins-sg` (for EC2 #1)
| Type | Port | Source | Why |
|---|---|---|---|
| SSH | 22 | My IP | You manage the server |
| Custom TCP | 8080 | My IP | Jenkins UI |

### `weather-app-sg` (for EC2 #2)
| Type | Port | Source | Why |
|---|---|---|---|
| SSH | 22 | My IP | You manage the server |
| SSH | 22 | **Custom → `jenkins-sg`** (pick the security group, not an IP) | Jenkins connects to the agent |
| HTTP | 80 | Anywhere (0.0.0.0/0) | The public API |

Using `jenkins-sg` as the source means "allow any instance in that group". It keeps working even if the Jenkins server's IP changes.

## Step 2: Launch the two instances

EC2 → **Launch instance**, twice, **in the same region and the same VPC** (the default VPC is fine):

| | EC2 #1 | EC2 #2 |
|---|---|---|
| Name | `jenkins` | `weather-app` |
| AMI | Ubuntu 24.04 | Ubuntu 24.04 |
| Type | t3.micro (Jenkins only, no builds) | t3.small, or t3.micro + swap (it compiles Go) |
| Key pair | your existing key | your existing key |
| Security group | `jenkins-sg` | `weather-app-sg` |
| Storage | 15 GB | 15 GB |

After launch, note down for each instance:
- **Public IPv4 address**: you use this from your Mac.
- **Private IPv4 address** (`172.31.x.x`): Jenkins uses this to reach EC2 #2. The private IP **doesn't change** when you stop and start the instance.

Set both to **Standard** CPU credits so long builds can't add charges: select the instance → Actions → Instance settings → **Change credit specification** → untick Unlimited.

## Step 3: Add swap on both instances (if t3.micro)

SSH into each one (`ssh -i ~/.ssh/your-key.pem ubuntu@<PUBLIC_IP>`) and run:

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
free -h        # Swap should show 2.0Gi
```

---

## Step 4: EC2 #1: install Docker and run Jenkins

SSH into **EC2 #1**:

```bash
sudo apt update
sudo apt install -y docker.io
sudo systemctl enable --now docker
sudo usermod -aG docker ubuntu
exit
```

Reconnect, then start Jenkins. It's the plain official image: no Docker CLI, no socket and no socat, because builds don't happen here.

```bash
docker run -d --name jenkins \
  --restart unless-stopped \
  -p 8080:8080 \
  -v jenkins_home:/var/jenkins_home \
  -e JAVA_OPTS="-Xmx512m" \
  jenkins/jenkins:lts-jdk21
```

Port 50000 isn't needed: SSH agents connect over port 22 instead.

Get the first-login password:

```bash
docker exec jenkins cat /var/jenkins_home/secrets/initialAdminPassword
```

Open `http://<EC2_1_PUBLIC_IP>:8080`, paste the password, choose **Install suggested plugins**, and create your admin user.

## Step 5: EC2 #2: prepare it as a build agent

SSH into **EC2 #2**. The agent needs Java (to run the Jenkins agent program), Docker and Git:

```bash
sudo apt update
sudo apt install -y openjdk-21-jre-headless docker.io docker-buildx git
sudo systemctl enable --now docker
```

The Java version must match the controller. The controller image is `lts-jdk21`, so install Java 21.

Create a dedicated `jenkins` user that Jenkins logs in as and that's allowed to use Docker:

```bash
sudo useradd -m -s /bin/bash jenkins
sudo usermod -aG docker jenkins
sudo mkdir -p /home/jenkins/.ssh /home/jenkins/agent
sudo chmod 700 /home/jenkins/.ssh
sudo chown -R jenkins:jenkins /home/jenkins
```

Check:

```bash
java -version                            # openjdk version "21..."
sudo -u jenkins docker run --rm hello-world
```

## Step 6: Create an SSH key for Jenkins → EC2 #2

This key is **separate from your `.pem`**. It's only for Jenkins, so you never upload your personal key to Jenkins.

**On EC2 #1**, generate the key pair:

```bash
ssh-keygen -t ed25519 -f ~/jenkins_agent_key -N "" -C "jenkins-agent"
cat ~/jenkins_agent_key.pub        # copy this whole line
```

**On EC2 #2**, authorize it for the `jenkins` user. Paste the line you copied in place of `<PASTE_PUBLIC_KEY_HERE>`:

```bash
echo '<PASTE_PUBLIC_KEY_HERE>' | sudo tee /home/jenkins/.ssh/authorized_keys
sudo chmod 600 /home/jenkins/.ssh/authorized_keys
sudo chown jenkins:jenkins /home/jenkins/.ssh/authorized_keys
```

**Back on EC2 #1**, test the connection over the private IP:

```bash
ssh -i ~/jenkins_agent_key jenkins@<EC2_2_PRIVATE_IP> 'java -version && docker ps'
```

Type `yes` to the host prompt. If it prints the Java version and an empty container list, the key, the user and the security group all work.

Print the **private** key; you'll paste it into Jenkins in the next step:

```bash
cat ~/jenkins_agent_key
```

## Step 7: Add the credentials in Jenkins

Manage Jenkins → **Credentials** → **System** → **Global credentials (unrestricted)** → **+ Add Credentials**.

### SSH key for the agent
| Field | Value |
|---|---|
| Kind | **SSH Username with private key** |
| ID | `weather-app-ssh` |
| Username | `jenkins` |
| Private Key | **Enter directly** → **Add** → paste everything from `cat ~/jenkins_agent_key`, including the `-----BEGIN` and `-----END` lines |

After saving, delete the private key file from EC2 #1, since Jenkins has its own copy now:

```bash
rm ~/jenkins_agent_key ~/jenkins_agent_key.pub
```

### The app's `.env`
| Field | Value |
|---|---|
| Kind | **Secret file** |
| ID | `weather-agent-env` |
| File | your local `.env` (press Cmd + Shift + . in the Mac file picker to show hidden files) |

The pipeline copies this file to EC2 #2 only while the Deploy stage runs, then deletes it.

## Step 8: Register EC2 #2 as a node in Jenkins

1. Check that the **SSH Build Agents** plugin is installed: Manage Jenkins → **Plugins** → **Installed plugins** → search "SSH Build Agents". If it's missing, install it from **Available plugins**.
2. Manage Jenkins → **Nodes** → **New Node**.
3. Node name: `weather-app-ec2`, type **Permanent Agent** → **Create**.
4. Fill in:

   | Field | Value |
   |---|---|
   | Number of executors | `1` |
   | Remote root directory | `/home/jenkins/agent` |
   | Labels | `weather-app` |
   | Usage | **Only build jobs with label expressions matching this node** |
   | Launch method | **Launch agents via SSH** |
   | Host | `<EC2_2_PRIVATE_IP>` (the `172.31.x.x` address) |
   | Advanced → JVM Options | `-Djava.io.tmpdir=/home/jenkins/agent/tmp` (newer Ubuntu's `/tmp` is a small RAM disk, and Jenkins takes the node offline if it has less than 1 GB free; create the folder first with `sudo -u jenkins mkdir -p /home/jenkins/agent/tmp` on EC2 #2) |
   | Credentials | `jenkins (weather-app-ssh)` |
   | Host Key Verification Strategy | **Manually trusted key Verification Strategy**, with **Require manual verification of initial connection** unticked (trusts the server on first connect) |

   ⚠️ The dropdown defaults to **Known hosts file Verification Strategy**. If you leave it, the connection fails with `No Known Hosts file was found at /var/jenkins_home/.ssh/known_hosts` and `Key exchange was not finished, connection is closed`. Change it, or create the file from EC2 #1:

   ```bash
   docker exec jenkins sh -c 'mkdir -p ~/.ssh && ssh-keyscan -H <EC2_2_PRIVATE_IP> >> ~/.ssh/known_hosts'
   ```

5. **Save**. Open the node and click **Log**: within a few seconds you should see `Agent successfully connected and online`.

### Stop builds from running on the Jenkins server itself
Manage Jenkins → **Nodes** → **Built-In Node** → **Configure** → set **Number of executors** to `0` → Save.

Now every build has to run on an agent. If the pipeline's label is wrong, the build waits in the queue instead of trying to build on EC2 #1, which has no Docker access.

## Step 9: Point the Jenkinsfile at the agent

In [Jenkinsfile](Jenkinsfile), change the agent line:

```groovy
pipeline {
    agent { label 'weather-app' }     // was: agent any
    ...
```

Nothing else changes. Every stage now runs on EC2 #2, so `docker build` and `docker run` use EC2 #2's Docker. Push it:

```bash
git add Jenkinsfile
git commit -m "Run pipeline on the weather-app EC2 agent"
git push
```

This change makes the pipeline require an agent with that label. If you still use the single-instance setup from [DEPLOYMENT.md](DEPLOYMENT.md), keep `agent any` there, or add the same label to that machine.

## Step 10: Create the pipeline job and build

Same as Step 7 of [DEPLOYMENT.md](DEPLOYMENT.md):

1. **New Item** → name `weather-agent` → **Pipeline**.
2. Definition: **Pipeline script from SCM**; SCM: **Git**.
3. Repository URL: `https://github.com/kshitijson/Weather-Agent.git`. Branch: `*/master`. Script Path: `Jenkinsfile`.
4. Save → **Build Now**.

The top of Console Output should show the build running on the agent:

```
Running on weather-app-ec2 in /home/jenkins/agent/workspace/weather-agent
```

## Step 11: Test

From your Mac:

```bash
curl -X POST http://<EC2_2_PUBLIC_IP>/chat \
  -H "Content-Type: application/json" \
  -d '{"input":"What is the weather in Pune?"}'
```

On EC2 #2:

```bash
docker ps                      # weather-agent should be Up, 0.0.0.0:80->8080
docker logs -f weather-agent
```

---

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Node offline: `Disk space is below threshold of 1.00 GiB ... left on /tmp` | Newer Ubuntu keeps `/tmp` in RAM (~450 MB on t3.micro). On EC2 #2 run `sudo -u jenkins mkdir -p /home/jenkins/agent/tmp`, then in the node's Configure → Launch method → **Advanced** → **JVM Options** set `-Djava.io.tmpdir=/home/jenkins/agent/tmp`. Disconnect → Launch agent. |
| Node log: `No Known Hosts file was found` + `Key exchange was not finished` | Host Key Verification Strategy is still the default "Known hosts file". Switch it to **Manually trusted key** (Step 8). |
| Node log: `Connection timed out` | `weather-app-sg` doesn't allow port 22 from `jenkins-sg`, or you used the public IP. Use the **private** IP. |
| Node log: `Authentication failed` | The public key isn't in `/home/jenkins/.ssh/authorized_keys` on EC2 #2, the permissions are wrong (`.ssh` must be 700, `authorized_keys` 600, both owned by `jenkins`), or the credential's username isn't `jenkins`. |
| Node log: `java: command not found` or `UnsupportedClassVersionError` | Java is missing on EC2 #2 or is the wrong version. Install `openjdk-21-jre-headless`. |
| Build stuck: `Waiting for next available executor on 'weather-app'` | The node is offline, or its label isn't exactly `weather-app`. Check Manage Jenkins → Nodes. |
| `permission denied ... docker.sock` in the build | The `jenkins` user isn't in the `docker` group on EC2 #2. Run `sudo usermod -aG docker jenkins`, then on the node page click **Disconnect** → **Launch agent** so it logs in again. |
| `Could not find credentials entry with ID 'weather-agent-env'` | The Secret file credential is missing or its ID is misspelled (Step 7). |
| Build fails with `exit code -1` / `Resuming build after Jenkins restart` | Out of memory, see the troubleshooting section in [DEPLOYMENT.md](DEPLOYMENT.md). With this setup it would be EC2 #2 that's low on memory: add swap there or use t3.small. |
| Node goes offline after you stop/start EC2 #2 | Expected. On the node page click **Launch agent**. The private IP stays the same, so no config change is needed. |

## Stopping everything to save money

1. Stop **EC2 #2** first, then **EC2 #1**. Stopped instances only cost their disk, which is covered by the Free Tier up to 30 GB total.
2. When starting again, start **EC2 #1**, then **EC2 #2**, then click **Launch agent** on the node page if it shows offline.
3. Both **public** IPs change on every stop/start. Use the new EC2 #1 IP for the Jenkins UI and the new EC2 #2 IP for the API. The private IP Jenkins uses doesn't change.
