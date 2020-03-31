package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/thoj/go-ircevent"
)

type otype int

const (
	MSG otype = iota
	JOIN
	NICK
)

type outgoing struct {
	t      otype
	target string
	msg    string
}

var dir string = "/tmp/9irc"

func verboseLog() *log.Logger {
	f, err := os.OpenFile(dir+"/log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatal(err)
	}
	return log.New(f, "", log.LstdFlags)
}
func rawFile() *os.File {
	f, err := os.OpenFile(dir+"/raw", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatal(err)
	}
	return f
}

var fhandles map[string]*os.File

func getFile(channel string) *os.File {
	if fhandles == nil {
		fhandles = make(map[string]*os.File)
	}
	f := fhandles[channel]
	if f == nil {
		var err error
		f, err = os.OpenFile(dir+"/"+channel, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			// TODO: Probably shouldn't be fatal in the long run
			log.Fatal(err)
		}
		fhandles[channel] = f
	}
	return f
}

//ircobj.SendRaw("<string>") //sends string to server. Adds \r\n
//ircobj.SendRawf("<formatstring>", ...) //sends formatted string to server.n
//ircobj.Join("<#channel> [password]")
//ircobj.Nick("newnick")
//ircobj.Privmsg("<nickname | #channel>", "msg") // sends a message to either a certain nick or a channel
//ircobj.Privmsgf(<nickname | #channel>, "<formatstring>", ...)
//ircobj.Notice("<nickname | #channel>", "msg")
//ircobj.Noticef("<nickname | #channel>", "<formatstring>", ...)

func main() {
	err := os.MkdirAll(dir, os.ModeDir|0775)
	if err != nil {
		log.Fatal(err)
	}
	msgs := make(chan outgoing, 10)
	raw := rawFile()
	ircobj := irc.IRC("testclient2", "jack_rabbit") //Create new ircobj
	ircobj.VerboseCallbackHandler = true
	ircobj.Log = verboseLog()
	//Set options
	ircobj.UseTLS = true //default is false
	ircobj.AddCallback("001", func(e *irc.Event) {
		//fmt.Printf("Event: %#v\n", e)
		ircobj.Join("##client.test.1")
		go listener(msgs)
		go handleOutgoing(ircobj, msgs)
	})
	ircobj.AddCallback("366", func(e *irc.Event) {
		//fmt.Printf("Event: %#v\n", e)
	})
	ircobj.AddCallback("PRIVMSG", func(e *irc.Event) {
		//fmt.Printf("PRIVMSG: %#v\n", e)
		channel := e.Arguments[0]
		f := getFile(channel)
		f.Write([]byte(fmt.Sprintf("[%s] %s: %s\n", time.Now().Format("01/02 03:04PM"), e.Nick, e.Arguments[1])))
	})
	ircobj.AddCallback("JOIN", func(e *irc.Event) {
		channel := e.Arguments[0]
		f := getFile(channel)
		f.Write([]byte(fmt.Sprintf("[%s] %s Joined %s\n", time.Now().Format("01/02 03:04PM"), e.Nick, channel)))
	})
//	ircobj.AddCallback("PING", func(e *irc.Event) {
//		ircobj.Pong()
//	})
	ircobj.AddCallback("*", func(e *irc.Event) {
		//fmt.Printf("ALLEvent: %#v\n", e)
		raw.Write([]byte(e.Raw + "\n"))
	})
	err = ircobj.Connect("chat.freenode.net:6697") //Connect to server
	if err != nil {
		log.Fatal(err)
	}

	//go pipeListener()

	//go listener(msgs)
	//go handleOutgoing(ircobj, msgs)
	ircobj.Loop()
}

func pipeListener() {
	os.Remove(dir + "/ctl")
	err := syscall.Mkfifo(dir+"/ctl", 0664)
	if err != nil {
		log.Fatal(err)
	}

	for {
		f, err := os.Open(dir + "/ctl")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		fmt.Println("Scanning")
		for scanner.Scan() {
			fmt.Println("Received scan.")
			fmt.Println(scanner.Text()) // Println will add back the final '\n'
		}
		fmt.Println("Finished Scanning.")
		if err := scanner.Err(); err != nil {
			//log.Fatal(err)
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}
	}
}

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

func parseIncoming(in string) (o outgoing, e error) {
	parts := strings.SplitN(in, " ", 3)
	switch parts[0] {
	case "msg":
		o.t = MSG
		if len(parts) != 3 {
			e = fmt.Errorf("Usage: msg [target] [msg]")
			return
		}
		o.target = parts[1]
		o.msg = parts[2]
	case "join":
		o.t = JOIN
		if len(parts) != 2 {
			e = fmt.Errorf("Usage: join [target]")
			return
		}
		o.target = parts[1]
	case "nick":
		o.t = NICK
		if len(parts) != 2 {
			e = fmt.Errorf("Usage: nick [new nick]")
			return
		}
		o.target = parts[1]
			
	default:
		e = fmt.Errorf("Invalid command %s.", parts[0])
	}
	return
}

func handleOutgoing(ircobj *irc.Connection, out <-chan outgoing) {
	for o := range out {
		switch o.t {
		case JOIN:
			ircobj.Join(o.target)
			fmt.Printf("Joining [%s]\n", o.target)
		case MSG:
			ircobj.Privmsg(o.target, o.msg)
			fmt.Printf("Sending [%s] -> [%s]\n", o.msg, o.target)
			f := getFile(o.target)
			f.Write([]byte(fmt.Sprintf("[%s] %s: %s\n", time.Now().Format("01/02 03:04PM"), ircobj.GetNick(), o.msg)))
		case NICK:
			ircobj.Nick(o.target)
			fmt.Printf("Changing nick to [%s]\n", o.target)
		}
	}
}
