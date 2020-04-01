package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"syscall"
)

func postfd(name string) (pipe *os.File, srvHandle *os.File, err error) {
	f1, f2, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	sf, err := os.OpenFile("/srv/"+name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|64|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, nil, err
	}

	_, err = sf.Write([]byte(fmt.Sprintf("%d", f2.Fd())))
	if err != nil {
		sf.Close()
		return nil, nil, err
	}
	f2.Close()
	return f1, sf, nil
}

func listener(msgs chan<- outgoing) {
	f, handle, err := postfd("9irc")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	defer handle.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fmt.Println(scanner.Text()) // Println will add back the final '\n'
		f.Write([]byte(fmt.Sprintf("Got: [%s]\n", scanner.Text())))
		out, err := parseIncoming(scanner.Text())
		if err != nil {
			f.Write([]byte(fmt.Sprintf("%s\n", err)))
			continue
		}
		select {
		case msgs <- out:
		default:
		}
		f.Write([]byte(fmt.Sprintf("%#v\n", out)))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
}
