package main

import (
	"bufio"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"github.com/rurreac/dump25/inbox"
	"gopkg.in/macaron.v1"
	"html/template"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultHttpPort = 10080
const defaultSmtpPort = "10025"
const smtpAuthentication = false
const defaultExpirationTime = 8
const defaultCleanUpInterval = 2 * defaultExpirationTime
const cacheFile = "dump25.gob"

var flagHttpPort int
var flagSmtpPort string
var flagExpTime int
var flagSmtpAuth bool
var flagCachePath string
var inboxCache *cache.Cache
var logD *log.Logger

func init() {
	flag.IntVar(&flagHttpPort, "httpPort", defaultHttpPort, "What port should the HTTP Server use.")
	flag.StringVar(&flagSmtpPort, "smtpPort", defaultSmtpPort, "What port should the fake SMTP Server use.")
	flag.IntVar(&flagExpTime, "expTime", defaultExpirationTime, "Expiration time (hours) of each Item in Queue.")
	flag.BoolVar(&flagSmtpAuth, "smtpAuth", smtpAuthentication, "Whatever if dump25 should ask for SMTP authentication.")
	flag.StringVar(&flagCachePath, "cachePath", "./", "Directory where Cache should be stored.")
	flag.Parse()

	gob.Register(&inbox.EmailCompose{})
	inboxCache = cache.New(defaultExpirationTime*time.Hour, defaultCleanUpInterval*time.Hour)
	logD = log.New(os.Stdout, "[dump25] ", 0)

	if _, err := os.Stat(flagCachePath + cacheFile); err == nil {
		logD.Println("Loading Cache File -", flagCachePath+cacheFile)
		if err = inboxCache.LoadFile(flagCachePath + cacheFile); err != nil {
			logD.Printf("Failed to load existing Cache File; %v, purging current Cache.\n", err)
			os.Remove(flagCachePath + cacheFile)
		}
	} else {
		if err := os.MkdirAll(flagCachePath, os.ModePerm); err != nil {
			logD.Panicf("Can't create Cache directory - %v\n", err)
		}
	}

}

func main() {
	m := macaron.Classic()
	m.Use(macaron.Static("public",
		macaron.StaticOptions{
			Prefix:      "public",
			SkipLogging: true,
		}))
	m.Use(macaron.Renderer(macaron.RenderOptions{
		Funcs: []template.FuncMap{map[string]interface{}{
			"InboxSize": func() int {
				inboxCache.DeleteExpired()
				return inboxCache.ItemCount()
			},
		}},
		Extensions: []string{".gohtml", ".tmpl", ".html"},
	}))
	m.Get("/", indexHandler)
	m.Get("/flush", flushInboxHandler)
	m.Get("/inbox", inboxHandler)
	m.Get("/inbox/:id", msgHandler)

	go m.Run("0.0.0.0", defaultHttpPort)

	smtpListener, _ := net.Listen("tcp", ":"+flagSmtpPort)
	defer smtpListener.Close()

	for true {
		conn, _ := smtpListener.Accept()
		go smtp(conn)
	}
}

func smtp(conn net.Conn) {
	var email inbox.EmailCompose
	var auth bool

	email.Id = uuid.New()
	email.Time = time.Now()
	email.SourceIP = conn.RemoteAddr().String()

	defer func() {
		_ = conn.Close()
		logD.Printf("Client %v disconnected.\n", email.SourceIP)
	}()

	io.WriteString(conn, "220 Dump25 Service.\r\n")

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := scanner.Text()
		cmdType1 := strings.Fields(line)
		cmdType2 := strings.SplitN(line, ":", 2)

		switch cmdType1[0] {
		case "EHLO":
			if flagSmtpAuth {
				io.WriteString(conn, "250 AUTH PLAIN LOGIN\r\n")
				io.WriteString(conn, "250 STARTTLS\r\n")
			} else {
				io.WriteString(conn, "250 Authentication disabled\r\n")
			}

		case "DATA":
			if requireAuth(conn, auth, email) {
				return
			}
			io.WriteString(conn, "354 Start mail input; end with <CRLF>.<CRLF>\r\n")
			var data string
			var boundaryFound bool
			for scanner.Scan() {
				dataLine := scanner.Text()
				if !boundaryFound {
					if ok, _ := regexp.Match(`^Content-Type: multipart/[[:alpha:]]+; boundary=`, []byte(dataLine)); ok {
						email.Boundary = strings.ReplaceAll(strings.SplitAfter(dataLine, "boundary=")[1], `"`, ``)
					}
				}
				if dataLine != "." {
					data += dataLine + "\n"
				} else {
					break
				}
			}
			email.Data = data
			jsonEmail, _ := json.Marshal(email)
			logD.Println(string(jsonEmail))
			inboxCache.Set(email.Id.String(), &email, cache.DefaultExpiration)
			if err := inboxCache.SaveFile(flagCachePath + cacheFile); err != nil {
				logD.Printf("Could not save email to cache file - %v\n", err)
				return
			}
			fmt.Fprintf(conn, "250 2.0.0 Ok: Email Id %v queued in dump25\r\n", email.Id.String())
		case "QUIT":
			io.WriteString(conn, "221 2.0.0 Bye\r\n")
		default:
			replacer := strings.NewReplacer("<", "", ">", "")
			switch cmdType2[0] {
			case "AUTH LOGIN":
				io.WriteString(conn, "344 VXNlcm5hbWU6;\r\n")
				scanner.Scan()
				if decoByte, err := base64.StdEncoding.DecodeString(scanner.Text()); err == nil {
					email.User = string(decoByte)
					io.WriteString(conn, "334 UGFzc3dvcmQ6;\r\n")
					scanner.Scan()
					scanner.Text()
					io.WriteString(conn, "235 Authentication succeeded")
					auth = true
				} else {
					fmt.Println("Unable to decode Username.", err)
				}
			case "MAIL FROM":
				if requireAuth(conn, auth, email) {
					return
				}
				email.From = replacer.Replace(cmdType2[1])
				io.WriteString(conn, "250 2.1.0 OK\r\n")
			case "RCPT TO":
				if requireAuth(conn, auth, email) {
					return
				}
				email.Rcpt = append(email.Rcpt, replacer.Replace(cmdType2[1]))
				io.WriteString(conn, "250 2.1.0 OK\r\n")
			default:
				return
			}
		}
	}
}

func requireAuth(conn net.Conn, authStatus bool, email inbox.EmailCompose) (require bool) {
	if flagSmtpAuth && !authStatus {
		io.WriteString(conn, "535 Incorrect authentication data\r\n")
		logD.Printf("Client %v did not perform authentication.", email.SourceIP)
		require = true
	}
	return
}

func inboxHandler(ctx *macaron.Context) {
	m := ctx.Req.Form
	ib := inbox.Get(inboxCache, m)
	ctx.JSON(200, &ib)
}

func msgHandler(ctx *macaron.Context) {
	if msg, err := inbox.GetMessage(inboxCache, ctx.Params("id")); err != nil {
		ctx.HTML(500, "message", err)
	} else {
		ctx.Req.ParseForm()
		if ctx.Req.Form.Get("plain") == "true" {
			ctx.PlainText(200, []byte(msg))
		}
		ctx.Write([]byte(msg))
	}
}

func indexHandler(ctx *macaron.Context) {
	ctx.Req.ParseForm()
	ib := inbox.Get(inboxCache, ctx.Req.Form)
	ctx.Data["inbox"] = ib
	ctx.HTML(200, "index")
}

func flushInboxHandler(ctx *macaron.Context) {
	logD.Printf("Cache flush requested from %v.\n", ctx.RemoteAddr())
	inboxCache.Flush()
	ctx.JSON(200, struct {
		QueueSize int `json:"queue_size"`
	}{
		QueueSize: inboxCache.ItemCount(),
	})
}
