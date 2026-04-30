#!/bin/sh
dnf install -y \
	git \
	golang \
	make

# Since go 1.25.x is not available in ubi8, let's go upstream
go install golang.org/dl/go1.25.9@latest
/root/go/bin/go1.25.9 download
rm /usr/bin/go
ln -s /root/go/bin/go1.25.9 /usr/bin/go
