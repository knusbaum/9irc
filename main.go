package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/knusbaum/go9p"
	"github.com/knusbaum/go9p/fs"
	"github.com/thoj/go-ircevent"
)

var dir string = "/tmp/9irc"
var ircFS *fs.FS
var streams map[string]fs.Stream

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

func verboseLog() *log.Logger {
	s := getFile("log")
	return log.New(s, "", log.LstdFlags)
}

func getFile(channel string) fs.Stream {
	if streams == nil {
		streams = make(map[string]fs.Stream)
	}
	s := streams[channel]
	if s == nil {
		var err error
		f, err := os.OpenFile(dir+"/"+channel, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			// TODO: Probably shouldn't be fatal in the long run
			log.Fatal(err)
		}
		f.Close()
		s, err = fs.NewSavedStream(dir + "/" + channel)
		if err != nil {
			log.Fatal(err)
		}
		streams[channel] = s
		ircFS.Root.AddChild(
			fs.NewStreamFile(
				ircFS.NewStat(channel, "glenda", "glenda", 0444),
				s,
			),
		)
	}
	return s
}

func setupStreams() {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range infos {
		getFile(info.Name())
	}
}

func main() {
	var username string
	u, err := user.Current()
	if err == nil {
		username = u.Username
	}

	dirFlag := flag.String("dir", "", "specifies the directory to which 9irc will log irc messages. (default \"/tmp/9irc\")")
	nick := flag.String("nick", "", "the nick that will be used.")
	user := flag.String("user", username, "the username to log into the server with.")
	server := flag.String("server", "chat.freenode.net:6697", "address (host and port) of the IRC server to connect to.")
	service := flag.String("svc", "9irc", "sets the service name that the 9p connection will be posted as. This will also change the log directory to /tmp/[svc] unless it is set with the dir flag.")
	auth := flag.Bool("auth", false, "This flag controls whether 9irc will require clients to authenticate oven 9p.")
	flag.Parse()

	dir = "/tmp/" + *service
	if *dirFlag != "" {
		dir = *dirFlag
	}
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

	if *auth {
		ircFS = fs.NewFS("glenda", "glenda", 0555, fs.WithAuth())
	} else {
		ircFS = fs.NewFS("glenda", "glenda", 0555)
	}
	ctlStream := fs.NewBlockingStream(10, true)
	ircFS.Root.AddChild(
		fs.NewStreamFile(
			ircFS.NewStat("ctl", "glenda", "glenda", 0666),
			ctlStream,
		),
	)

	setupStreams()

	msgs := make(chan outgoing, 10)
	raw := getFile("raw")
	ircobj := irc.IRC(*nick, *user) //Create new ircobj
	ircobj.VerboseCallbackHandler = true
	ircobj.Log = verboseLog()
	ircobj.UseTLS = true //default is false
	ircobj.AddCallback("001", func(e *irc.Event) {
		for k := range streams {
			if strings.HasPrefix(k, "#") {
				ircobj.Join(k)
			}
		}
	})
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

	go listener9p(ctlStream, msgs)
	go handleOutgoing(ircobj, msgs)
	go ircobj.Loop()
	log.Println(go9p.PostSrv("9irc", ircFS.Server()))
}

func listener9p(s fs.BiDiStream, msgs chan<- outgoing) {
	scanner := bufio.NewScanner(s)
	for scanner.Scan() {
		fmt.Println(scanner.Text()) // Println will add back the final '\n'
		s.Write([]byte(fmt.Sprintf("Got: [%s]\n", scanner.Text())))
		out, err := parseIncoming(scanner.Text())
		if err != nil {
			s.Write([]byte(fmt.Sprintf("%s\n", err)))
			continue
		}
		select {
		case msgs <- out:
		default:
		}
		s.Write([]byte(fmt.Sprintf("%#v\n", out)))
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
