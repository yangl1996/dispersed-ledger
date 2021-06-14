package main

import (
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"strconv"
)

func getPublicKey(path string) ([]byte, error) {
	key, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	return ssh.MarshalAuthorizedKey(signer.PublicKey()), err
}

func loadSSHKey(path string) (ssh.AuthMethod, error) {
	key, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func connectSSH(user, addr string, port int, key string) (*ssh.Client, error) {
	au, err := loadSSHKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig {
		User: user,
		Auth: []ssh.AuthMethod {
			au,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	fulladdr := addr + ":" + strconv.Itoa(port)
	client, err := ssh.Dial("tcp", fulladdr, config)
	return client, err
}

