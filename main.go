package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/thoj/go-ircevent"
)

type otype int

const (
	MSG otype = iota
	JOIN
	PART
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

func main() {
	var username string
	u, err := user.Current()
	if err == nil {
		username = u.Username
	}

	dirFlag := flag.String("dir", "/tmp/9irc", "specifies the directory to which 9irc will write irc messages.")
	nick := flag.String("nick", "", "the nick that will be used.")
	user := flag.String("user", username, "the username to log into the server with.")
	server := flag.String("server", "chat.freenode.net:6697", "address (host and port) of the IRC server to connect to.")
	flag.Parse()

	dir = *dirFlag
	if *nick == "" {
		log.Print("nick not provided.")
		flag.Usage()
		os.Exit(1)
	}
	if *user == "" {
		log.Print("user not provided.")
		flag.Usage()
		os.Exit(1)
	}

	err = os.MkdirAll(dir, os.ModeDir|0775)
	if err != nil {
		log.Fatal(err)
	}
	msgs := make(chan outgoing, 10)
	raw := rawFile()
	ircobj := irc.IRC("testclient2", "jack_rabbit") //Create new ircobj
	ircobj.VerboseCallbackHandler = true
	ircobj.Log = verboseLog()
	ircobj.UseTLS = true //default is false
	//	ircobj.AddCallback("001", func(e *irc.Event) {
	//		ircobj.Join("##client.test.1")
	//		go listener(msgs)
	//		go handleOutgoing(ircobj, msgs)
	//	})
	//	ircobj.AddCallback("366", func(e *irc.Event) {
	//		fmt.Printf("Event: %#v\n", e)
	//	})
	ircobj.AddCallback("PRIVMSG", func(e *irc.Event) {
		channel := e.Arguments[0]
		f := getFile(channel)
		f.Write([]byte(fmt.Sprintf("[%s] %s: %s\n", time.Now().Format("01/02 03:04PM"), e.Nick, e.Arguments[1])))
	})
	ircobj.AddCallback("JOIN", func(e *irc.Event) {
		channel := e.Arguments[0]
		f := getFile(channel)
		f.Write([]byte(fmt.Sprintf("[%s] %s Joined %s\n", time.Now().Format("01/02 03:04PM"), e.Nick, channel)))
	})
	ircobj.AddCallback("*", func(e *irc.Event) {
		raw.Write([]byte(e.Raw + "\n"))
	})
	err = ircobj.Connect(*server) //Connect to server
	if err != nil {
		log.Fatal(err)
	}

	go listener(msgs)
	go handleOutgoing(ircobj, msgs)
	ircobj.Loop()
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
	case "part":
		o.t = PART
		if len(parts) != 2 {
			e = fmt.Errorf("Usage: part [target]")
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
		case PART:
			ircobj.Part(o.target)
			fmt.Printf("Parting [%s]\n", o.target)
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
