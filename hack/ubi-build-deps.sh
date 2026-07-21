#!/bin/sh
dnf install -y \
	git \
	golang \
	make

# Since go 1.26.x is not available in ubi8, let's go upstream
go install golang.org/dl/go1.26.4@latest
/root/go/bin/go1.26.4 download
rm /usr/bin/go
ln -s /root/go/bin/go1.26.4 /usr/bin/go
