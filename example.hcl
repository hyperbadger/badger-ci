settings {
        pathto = "/code"
        localweb = "http://{IP}:80/files"
        localpath = "/var/www/html/files/"
        localinterface = "eth0"
        gitlabpath = "https://gitlab.com/hyperbadger/ci/library/-/archive/{VERSION}/library-{VERSION}.zip"
        githubpath = "https://github.com/hyperbadger/ci/library/archive/refs/heads/{VERSION}.zip"
        default_container = "somecontainerwithpython:latest"
        environment "local" {
            region = "global"
            priority = "50"
            datacenter = "dc1"
        }
        environment "remote" {
            region = "global"
            priority = "50"
            datacenter = "dc1"
        }
}

stage "test" "formatting" {
    step "runblack" {
        driver "docker" {
            container = "mercutiodesign/docker-black:latest"
        }
        command = ["/usr/local/bin/black ."]
        environments = ["local","remote"]
        pathto = "/local/code/"
        workdir = "/local/code/index-api/"
    }
    step "runpylint" {
        driver "docker" {
            container = "cytopia/pylint:latest"
        }
        command = ["pylint ."]
        environments = ["local","remote"]
        pathto = "/local/usr/src/"
        workdir = "/local/usr/src/index-api/"
    }
}

stage "test" "testing" {
    step "unittest" {
        driver "docker" {
            container = "safesecurity/pytest:latest"
        }
        command = ["pip3 install -r requirements.txt",
        "pytest tests"]
        environments = ["local"]
        pathto = "/local/usr/src/"
        workdir = "/local/usr/src/index-api/"
    }
    step "functest" {
        driver "docker" {
            container = "safesecurity/pytest:latest"
        }
        command = ["ls -la",
        "pip3 install -r requirements.txt",
        "pytest tests"]
        deployment = "functest_deployment"
        environments = ["remote"]
        pathto = "/local/usr/src/"
        workdir = "/local/usr/src/index-api/"
    }
}

deployment "functest_deployment" {
    pack = "somepack.nomad"
}