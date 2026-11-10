# Deploying Weather Agent to AWS EC2 (Docker + Jenkins)

## How it fits together

```
EC2 (Ubuntu)
 ├─ Docker
 │   ├─ jenkins container  (port 8080) ── uses host's /var/run/docker.sock
 │   └─ weather-agent container (host port 80 → container 8080)  ← started by Jenkins
```

Jenkins runs in a container but uses the EC2 host's Docker. When it runs `docker run`, the app starts on the host next to Jenkins, not inside the Jenkins container.

Files in this repo used for deployment:

- [Dockerfile](Dockerfile): builds the Go app and runs it in a small Alpine image on port 8080.
- [.dockerignore](.dockerignore): keeps `.env` and `.git` out of the image.
- [Jenkinsfile](Jenkinsfile): checks out the code, builds the image, replaces the running container, and prunes old images. The `.env` comes from a Jenkins credential, never from git.

Before the first push, check the image builds locally:

```bash
docker build -t weather-agent .
```

---

## Step 1: Launch the EC2 instance

1. In the AWS Console, go to EC2 → Launch instance.
2. Choose **Ubuntu 24.04** and a Free Tier instance (see [Free Tier](#free-tier) below): **t3.small** for accounts created on or after July 15, 2025, otherwise **t3.micro** plus swap.
3. Create or choose a key pair (`.pem` file).
4. Security group inbound rules:

   | Port | Source    | Why         |
   |------|-----------|-------------|
   | 22   | My IP     | SSH         |
   | 8080 | My IP     | Jenkins UI  |
   | 80   | 0.0.0.0/0 | Your API    |

5. Storage: 20 GB.

## Step 2: SSH into EC2 and install Docker

### 2a. Find your instance's public IP

EC2 → Instances → select your instance → copy **Public IPv4 address** from the Details tab.

### 2b. Prepare the key file (on your Mac, once)

```bash
mkdir -p ~/.ssh
mv ~/Downloads/your-key.pem ~/.ssh/
chmod 400 ~/.ssh/your-key.pem      # SSH refuses keys that other users can read
```

### 2c. SSH in

```bash
ssh -i ~/.ssh/your-key.pem ubuntu@<EC2_PUBLIC_IP>
```

- Ubuntu images log in as `ubuntu`. Amazon Linux images use `ec2-user`.
- On the first connection, type `yes` to trust the host.
- Your prompt changes to something like `ubuntu@ip-172-31-xx-xx:~$`. Every command after this runs **on EC2**.

**Shortcut (optional):** add this to `~/.ssh/config` on your Mac:

```
Host weather-ec2
    HostName <EC2_PUBLIC_IP>
    User ubuntu
    IdentityFile ~/.ssh/your-key.pem
```

After that, `ssh weather-ec2` is enough.

**No terminal handy?** EC2 → select instance → **Connect** → **EC2 Instance Connect** → Connect opens a shell in the browser.

### 2d. Install Docker and Git (on EC2)

```bash
sudo apt update
sudo apt install -y docker.io docker-buildx git
sudo systemctl enable --now docker      # start Docker now and on every boot
sudo usermod -aG docker ubuntu          # run docker without sudo
exit                                    # log out so the group change applies
```

`docker-buildx` installs BuildKit, Docker's current image builder. Without it, every `docker build` prints:

```
DEPRECATED: The legacy builder is deprecated and will be removed in a future release.
            Install the buildx component to build images with BuildKit
```

If Docker is already installed without it, add it on its own:

```bash
sudo apt install -y docker-buildx
```

### 2e. Reconnect and verify

```bash
ssh -i ~/.ssh/your-key.pem ubuntu@<EC2_PUBLIC_IP>

docker --version
docker buildx version                   # confirms BuildKit is installed
docker run --rm hello-world             # should print "Hello from Docker!"
git --version
```

### SSH troubleshooting

| Error | Fix |
|---|---|
| `Permission denied (publickey)` | Wrong user (use `ubuntu`) or wrong key file. |
| `UNPROTECTED PRIVATE KEY FILE` | Run `chmod 400 ~/.ssh/your-key.pem`. |
| `Connection timed out` | The security group isn't allowing port 22 from your current IP. Edit the inbound rule; your IP may have changed. |
| `permission denied ... docker.sock` | You didn't log out and back in after `usermod`. |

### Getting a shell inside a container (on EC2)

Containers aren't reached over SSH. Use `docker exec` from the EC2 shell:

```bash
docker ps                                   # list running containers
docker exec -it jenkins bash                # shell inside Jenkins
docker exec -it weather-agent sh            # shell inside the app (Alpine has sh, not bash)
docker logs -f weather-agent                # follow app logs
exit                                        # leave the container shell
```

## Step 3: Build a Jenkins image that has the Docker CLI

The stock `jenkins/jenkins` image has no `docker` command, so the pipeline's `docker build` would fail. On EC2:

```bash
mkdir ~/jenkins && cd ~/jenkins
cat > Dockerfile <<'EOF'
FROM jenkins/jenkins:lts
USER root
COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker:27-cli /usr/local/libexec/docker/cli-plugins /usr/local/libexec/docker/cli-plugins
USER jenkins
EOF

docker build -t my-jenkins .
```

## Step 4: Run Jenkins

```bash
docker run -d --name jenkins \
  --restart unless-stopped \
  -p 8080:8080 -p 50000:50000 \
  -v jenkins_home:/var/jenkins_home \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $(stat -c '%g' /var/run/docker.sock) \
  my-jenkins
```

- The `docker.sock` mount lets Jenkins control the host's Docker.
- `--group-add` gives the Jenkins user permission to use that socket.
- The `jenkins_home` volume keeps your Jenkins settings and jobs if the container is recreated.

Get the first-login password:

```bash
docker exec jenkins cat /var/jenkins_home/secrets/initialAdminPassword
```

Open `http://<EC2_PUBLIC_IP>:8080`, paste the password, choose **Install suggested plugins**, and create an admin user.

## Step 5: Add your `.env` as a Jenkins secret

1. Go to Manage Jenkins → Credentials → System → Global → **Add Credentials**.
2. Kind: **Secret file**.
3. File: upload your local `.env`.
4. ID: `weather-agent-env`. It must match exactly because the Jenkinsfile uses this ID.

You don't need to change `PORT` in the file. The pipeline forces the app onto 8080 inside the container and maps it to port 80 on the host, so it won't clash with Jenkins.

## Step 6: Push the deployment files to GitHub

```bash
git add Dockerfile .dockerignore Jenkinsfile DEPLOYMENT.md main.go tools/tools.go
git commit -m "Add Docker and Jenkins deployment"
git push
```

## Step 7: Create the pipeline job

1. In Jenkins, click New Item, name it `weather-agent`, and choose **Pipeline**.
2. Under Pipeline, set Definition to **Pipeline script from SCM**.
3. SCM: Git. Repository URL: `https://github.com/kshitijson/Weather-Agent.git`.
   - If the repo is private, add a GitHub username and Personal Access Token as a credential and select it here.
4. Branch: `*/master`. Script Path: `Jenkinsfile`.
5. Save, then click **Build Now**.

## Step 8: Test it

```bash
curl -X POST http://<EC2_PUBLIC_IP>/chat \
  -H "Content-Type: application/json" \
  -d '{"input":"What is the weather in Pune?"}'
```

On EC2, view app logs with:

```bash
docker logs -f weather-agent
```

## Optional: deploy on every push

1. In the job, tick **GitHub hook trigger for GITScm polling**.
2. In GitHub, go to repo Settings → Webhooks and add `http://<EC2_PUBLIC_IP>:8080/github-webhook/` with content type `application/json`.
3. Port 8080 must accept GitHub's IP ranges for this to work. The simpler option is **Poll SCM** with `H/5 * * * *`, which checks for changes every 5 minutes.

## Free Tier

| AWS account created | Free instance types | Pick |
|---|---|---|
| On or after July 15, 2025 | t3.micro, t3.small, t4g.micro, t4g.small, c7i-flex.large, m7i-flex.large (paid from credits, 6 months) | **t3.small** (2 GB) |
| Before July 15, 2025 | t2.micro, t3.micro (750 hrs/month, 12 months) | **t3.micro** (1 GB) + swap |

Avoid t4g instances. They use ARM processors and would need extra changes to the build.

**On t3.micro**, add swap before starting Jenkins:

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

and add `-e JAVA_OPTS="-Xmx512m"` to the Jenkins `docker run` in Step 4.

**Staying at $0:**

- Disk: 30 GB is free; this guide uses 20 GB.
- Public IP: the free tier covers one public IPv4 address while the instance runs. An Elastic IP that isn't attached to a running instance is billed.
- Free hours are a monthly total across all instances, so run only one.
- Create a **Zero spend budget** (Billing → Budgets) to get an email as soon as anything costs money.
- Terminate the instance when you're done. A stopped instance still keeps its disk.

Check which instance types are free on your account:

```bash
aws ec2 describe-instance-types --filters Name=free-tier-eligible,Values=true --query "InstanceTypes[].InstanceType"
```

## Things to watch

- **Public IP changes:** the EC2 public IP changes when you stop and start the instance. Attach an Elastic IP to keep it fixed.
- **Access risk:** anyone who can use Jenkins effectively has root on the EC2 host, because of the socket mount. Keep port 8080 limited to your IP.

## Troubleshooting: build fails with "Jenkins restart" / exit code -1

### Symptom

```
#14 [builder 6/6] RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o weather-agent .
Resuming build at ... after Jenkins restart
wrapper script does not seem to be touching the log file ...
ERROR: script returned exit code -1
```

During `go build`, Jenkins went down and came back up. `--restart=on-failure` restarted it, but the shell step it was running had already been killed. That's why the result is `exit code -1`.

**The likely cause is running out of memory.** Compiling Go, especially with the Gemini SDK, needs several hundred MB of RAM. Jenkins (Java) is already using a lot. When the server runs out, Linux kills the biggest process, which is Jenkins.

### 1. Confirm it (on EC2)

```bash
free -h
sudo dmesg -T | grep -i -E "out of memory|killed process" | tail -5
```

If you see `Out of memory: Killed process ... (java)`, that confirms it. Also check the `Swap:` line from `free -h`: if it shows `0B`, you have no swap.

### 2. Add 2 GB of swap

Swap is disk space Linux can use as extra memory:

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
free -h        # Swap should now show 2.0Gi
```

### 3. Cap Jenkins's memory

In `start.sh`, add this line to the Jenkins `docker run`, for example right after the `--env DOCKER_HOST` line:

```bash
  --env JAVA_OPTS="-Xmx512m" \
```

Then recreate the Jenkins container. Your jobs are kept in the `jenkins-data` volume:

```bash
docker rm -f jenkins-blueocean
# run only the Jenkins docker run command from start.sh again (no need to rebuild the image)
```

### 4. Build again

Click **Build Now**. To watch memory while it builds, run this in a second SSH window:

```bash
watch -n 2 free -h
```

Press Ctrl + C to stop watching. That's safe here, because it only stops `watch`.

### If it still crashes

- **Check your instance type** under EC2 → Instances → Instance type. On t3.micro (1 GB), this setup is very tight even with swap. If your account was created after July 15, 2025, **t3.small** (2 GB) is also Free Tier eligible: stop the instance, choose **Actions → Instance settings → Change instance type → t3.small**, then start it. Your public IP will change.
- **Make the Go build use less memory:** in the Weather Agent [Dockerfile](Dockerfile), change the build line to compile one package at a time:

  ```dockerfile
  RUN CGO_ENABLED=0 GOOS=linux go build -p 1 -ldflags="-s -w" -o weather-agent .
  ```

  It builds more slowly but needs much less memory.
