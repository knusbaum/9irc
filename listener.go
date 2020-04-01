// +build !plan9

package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
)

func listener(msgs chan<- outgoing) {
	os.Remove(dir + "/ctl")
	l, err := net.Listen("unix", dir+"/ctl")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn, msgs)
	}
}

func handleConn(c net.Conn, msgs chan<- outgoing) {
	scanner := bufio.NewScanner(c)
	for scanner.Scan() {
		fmt.Println(scanner.Text()) // Println will add back the final '\n'
		c.Write([]byte(fmt.Sprintf("Got: [%s]\n", scanner.Text())))
		out, err := parseIncoming(scanner.Text())
		if err != nil {
			c.Write([]byte(fmt.Sprintf("%s\n", err)))
			continue
		}
		select {
		case msgs <- out:
		default:
		}
		c.Write([]byte(fmt.Sprintf("%#v\n", out)))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
}
