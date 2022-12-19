settings {
        pathto = "/code"
        localweb = "http://{IP}:80/files"
        localpath = "/var/www/html/files/"
        localinterface = "eth0"
        gitlabpath = "https://gitlab.com/hyperbadger/ci/library/-/archive/{VERSION}/library-{VERSION}.zip"
        githubpath = "https://github.com/hyperbadger/ci/library/archive/refs/heads/{VERSION}.zip"
        default_container = "somecontainerwithpython:latest"
        environments = ["local","remote"]
        region = "global"
        priority = "50"
        datacenter = "dc1"
}

stage "test" "formatting" {
    step "runblack" {
        driver "docker" {
            container = "mercutiodesign/docker-black:latest"
        }
        command = ["/usr/local/bin/black", "/code/"]
        environments = ["local","remote"]
        pathto = "/code"
    }
    step "runpylint" {
        driver "docker" {
            container = "cytopia/pylint:latest"
        }
        command = ["pylint", "test"]
        environments = ["local","remote"]
        pathto = "/usr/src"
    }
}

stage "test" "testing" {
    step "unittest" {
        driver "docker" {
            container = "safesecurity/pytest:latest"
        }
        command = ["pytest","/usr/src"]
        environments = ["local"]
        pathto = "/usr/src"
    }
    step "functest" {
        driver "docker" {
            container = "safesecurity/pytest:latest"
        }
        command = ["pytest","/usr/src"]
        deployment = "functest_deployment"
        environments = ["remote"]
        pathto = "/usr/src"
    }
}

deployment "functest_deployment" {
    pack = "somepack.nomad"
}