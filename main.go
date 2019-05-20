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
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type emailCompose struct {
	Id       uuid.UUID `json:"id"`
	Time     time.Time `json:"time"`
	SourceIP string    `json:"srcIp"`
	User     string    `json:"user"`
	From     string    `json:"from"`
	Rcpt     []string  `json:"rcpt"`
	Data     string    `json:"data"`
}

type queue []*emailCompose

const defaultHttpPort = "10080"
const defaultSmtpPort = "10025"
const smtpAuthentication = false
const defaultExpirationTime = 8
const defaultCleanUpInterval = 2 * defaultExpirationTime
const cacheFile = "dump25.gob"

var flagHttpPort string
var flagSmtpPort string
var flagExpTime int
var flagSmtpAuth bool
var flagCachePath string
var queueCache *cache.Cache

func init() {
	flag.StringVar(&flagHttpPort, "httpPort", defaultHttpPort, "What port should the HTTP Server use.")
	flag.StringVar(&flagSmtpPort, "smtpPort", defaultSmtpPort, "What port should the fake SMTP Server use.")
	flag.IntVar(&flagExpTime, "expTime", defaultExpirationTime, "Expiration time (hours) of each Item in Queue.")
	flag.BoolVar(&flagSmtpAuth, "smtpAuth", smtpAuthentication, "Whatever if dump25 should ask for SMTP authentication.")
	flag.StringVar(&flagCachePath, "cachePath", "./", "Directory where Cache should be stored.")
	flag.Parse()

	queueCache = cache.New(defaultExpirationTime*time.Hour, defaultCleanUpInterval*time.Hour)

	gob.Register(&emailCompose{})

	if _, err := os.Stat(flagCachePath + cacheFile); err == nil {
		log.Println("Loading Cache File -", flagCachePath+cacheFile)
		if err = queueCache.LoadFile(flagCachePath + cacheFile); err != nil {
			log.Println("Failed to load existing Cache File;", err)
		}
	} else {
		if err := os.MkdirAll(flagCachePath, os.ModePerm); err != nil {
			log.Panicf("Can't create Cache directory - %v\n", err)
		}
	}

}

func dumpJsonQueue(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Panicln(err)
	}
	w.Header().Set("Content-Type", "application/json; charset:utf-8")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(dumpCache(queueCache, r))
	if err != nil {
		log.Panicln(err)
	}
}

func dumpCache(c *cache.Cache, r *http.Request) (tmpQ queue) {
	items := c.Items()
	ip := r.Form.Get("ip")
	from := r.Form.Get("from")
	user := r.Form.Get("user")
	for _, item := range items {
		filter := true
		if ip != "" {
			if ok, _ := regexp.Match(ip, []byte(item.Object.(*emailCompose).SourceIP)); !ok {
				filter = false
			}
		}
		if from != "" && filter {
			if ok, _ := regexp.Match(from, []byte(item.Object.(*emailCompose).From)); ok {
				filter = true
			} else {
				filter = false
			}
		}
		if user != "" && filter {
			if user == item.Object.(*emailCompose).User {
				filter = true
			} else {
				filter = false
			}
		}
		if filter {
			tmpQ = append(tmpQ, item.Object.(*emailCompose))
		}
	}
	return
}

func flushCache(w http.ResponseWriter, r *http.Request) {
	log.Printf("Cache flush requested from %v.\n", r.RemoteAddr)
	queueCache.Flush()
	w.Header().Set("Content-Type", "application/json; charset:utf-8")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(
		struct {
			QueueSize int `json:"queue_size"`
		}{

			QueueSize: queueCache.ItemCount(),
		})
	if err != nil {
		log.Panic(err)
	}
}

func requireAuth(conn net.Conn, authStatus bool, email emailCompose) (require bool) {
	if flagSmtpAuth && !authStatus {
		io.WriteString(conn, "535 Incorrect authentication data\r\n")
		log.Printf("Client %v did not perform authentication.", email.SourceIP)
		require = true
	}
	return
}

func main() {
	http.HandleFunc("/", dumpJsonQueue)
	http.HandleFunc("/flush", flushCache)
	go http.ListenAndServe(":"+flagHttpPort, nil)

	smtpListener, _ := net.Listen("tcp", ":"+flagSmtpPort)
	defer smtpListener.Close()

	for true {
		conn, _ := smtpListener.Accept()
		go smtp(conn)
	}
}

func smtp(conn net.Conn) {
	var email emailCompose
	var auth bool

	email.Id = uuid.New()
	email.Time = time.Now()
	email.SourceIP = conn.RemoteAddr().String()

	defer func() {
		conn.Close()
		log.Printf("Client %v disconnected.\n", email.SourceIP)
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
			for scanner.Scan() {
				dataLine := scanner.Text()
				if dataLine != "." {
					data += dataLine + "\n"
				} else {
					break
				}
			}
			email.Data = data
			jsonEmail, _ := json.Marshal(email)
			log.Println(string(jsonEmail))
			queueCache.Set(email.Id.String(), &email, cache.DefaultExpiration)
			if err := queueCache.SaveFile(flagCachePath + cacheFile); err != nil {
				log.Printf("Could not save email to cache file - %v\n", err)
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
