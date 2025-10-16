#!/bin/sh
dnf install -y \
	git \
	golang \
	make

# Since go 1.24.x is not available in ubi8, let's go upstream
go install golang.org/dl/go1.24.6@latest
/root/go/bin/go1.24.6 download
rm /usr/bin/go
ln -s /root/go/bin/go1.24.6 /usr/bin/go
