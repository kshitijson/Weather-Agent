pipeline {
    agent any

    environment {
        IMAGE     = "weather-agent"
        CONTAINER = "weather-agent"
        HOST_PORT = "80"     // public port on EC2 (Jenkins itself uses 8080)
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Build Image') {
            steps {
                sh 'docker build -t $IMAGE:$BUILD_NUMBER -t $IMAGE:latest .'
            }
        }

        stage('Deploy') {
            steps {
                // "weather-agent-env" = Jenkins "Secret file" credential holding your .env
                withCredentials([file(credentialsId: 'weather-agent-env', variable: 'ENV_FILE')]) {
                    sh '''
                        docker rm -f $CONTAINER || true
                        docker run -d \
                          --name $CONTAINER \
                          --restart unless-stopped \
                          --env-file "$ENV_FILE" \
                          -e PORT=8080 \
                          -p $HOST_PORT:8080 \
                          $IMAGE:latest
                    '''
                }
            }
        }

        stage('Cleanup') {
            steps {
                sh 'docker image prune -f'
            }
        }
    }
}
