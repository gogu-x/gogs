pipeline {
    agent any
    options { disableConcurrentBuilds() }
    parameters {
        string(name: 'BRANCH', defaultValue: 'main', trim: true, description: '要构建的 Git 分支名称')
    }
    environment { IMAGE = 'xiaoguyun/gogs-all' }
    stages {
        stage('检出代码') {
            steps {
                deleteDir()
                git branch: params.BRANCH, url: 'https://github.com/gogu-x/gogs.git'
                script {
                    env.PREFIX = params.BRANCH.replaceAll('[^A-Za-z0-9_.-]', '-').replaceAll('^[.-]+', 'b-').take(98)
                    env.VERSION = env.PREFIX + '-' + sh(script: 'git rev-parse --short=12 HEAD', returnStdout: true).trim() + '-' + env.BUILD_NUMBER
                }
            }
        }
        stage('构建并归档单镜像') {
            steps {
                sh '''
                    set -eu
                    docker build -f all.Dockerfile -t "$IMAGE:$VERSION" .
                    for svc in battle game gate pf platform; do
                        docker run --rm --network none "$IMAGE:$VERSION" --service-type "$svc" --help >/dev/null
                    done
                    docker save -o "gogs-$VERSION.tar" "$IMAGE:$VERSION"
                    gzip "gogs-$VERSION.tar"
                '''
                archiveArtifacts artifacts: 'gogs-*.tar.gz', fingerprint: true
            }
        }
        stage('推送到 Docker Hub') {
            steps {
                withCredentials([usernamePassword(credentialsId: 'dockerhub-xiaoguyun', usernameVariable: 'DH_USER', passwordVariable: 'DH_PASS')]) {
                    sh '''
                        set -eu
                        export DOCKER_CONFIG="$(mktemp -d)"
                        trap 'rm -rf "$DOCKER_CONFIG"' EXIT
                        printf '%s' "$DH_PASS" | docker login -u "$DH_USER" --password-stdin
                        docker push "$IMAGE:$VERSION"
                        docker tag "$IMAGE:$VERSION" "$IMAGE:$PREFIX-latest"
                        docker push "$IMAGE:$PREFIX-latest"
                    '''
                }
            }
        }
    }
    post {
        always {
            sh 'if [ -n "$VERSION" ]; then docker rmi "$IMAGE:$VERSION" "$IMAGE:$PREFIX-latest" || true; fi'
            deleteDir()
        }
    }
}
