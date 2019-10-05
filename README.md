IMAP facade for OpenProject
===========================

This is an IMAP facade for OpenProjects.

To allow users to view their work packages as messages from an email client.

## Quick Start

#### Download

    git clone https://github.com/lectio/imap-facade-openproject.git

#### Create config `conf/imap.toml`

    cp conf/imap.toml.example conf/imap.toml

Make sure to change the `base` variable to your OpenProject URL.

#### Build and run

    go build main.go
    ./main run

#### Connect with IMAP client

Use your OpenProject API Key as the password when setting up the IMAP client.

