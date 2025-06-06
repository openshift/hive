#!/bin/sh
dnf install -y \
	git \
	golang \
	make

# Since go 1.23.x is not available in ubi8, let's go upstream
go install golang.org/dl/go1.23.6@latest
/root/go/bin/go1.23.6 download
rm /usr/bin/go
ln -s /root/go/bin/go1.23.6 /usr/bin/go
