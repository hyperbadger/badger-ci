#!/bin/bash

sudo /usr/sbin/nginx
sudo nomad agent -dev -bind 0.0.0.0 -log-level INFO &
sudo dockerd &
