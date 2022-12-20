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