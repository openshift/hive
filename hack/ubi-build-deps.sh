#!/bin/sh
dnf install -y \
	git \
	golang \
	make

# Since go 1.26.x is not available in ubi8, let's go upstream
go install golang.org/dl/go1.26.3@latest
/root/go/bin/go1.26.3 download
rm /usr/bin/go
ln -s /root/go/bin/go1.26.3 /usr/bin/go
