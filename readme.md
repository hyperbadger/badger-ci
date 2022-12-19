## BADGER CI

Using a HCL CI file you can run this independent of your GIT environment in either a local or remote context. all you need is a nomad orchestration engine.

### BOOTSTRAP AND BUILD

```
go mod tidy
go build badger/paws.go
```

### PREREQS
install nginx, docker, nomad (follow the guides)

```
./startup.sh 
```

### EXAMPLE COMMAND
paws example.hcl run test

### KNOWN ISSUES
- Haven't tested or implemented remote CI i.e. gitlab/github.
- Environments are ignored so no filtering local/remote when running.

### EXAMPLE HCL CONFIG

```
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
```