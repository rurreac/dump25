package inbox

import (
	"bufio"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"io/ioutil"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"
)

type EmailCompose struct {
	Id       uuid.UUID `json:"id"`
	Time     time.Time `json:"time"`
	SourceIP string    `json:"srcIp"`
	User     string    `json:"user"`
	From     string    `json:"from"`
	Rcpt     []string  `json:"rcpt"`
	Boundary string    `json:"boundary"`
	Data     string    `json:"data"`
}

type Inbox []*EmailCompose

func (ib Inbox) Len() int {
	return len(ib)
}

func (ib Inbox) Less(i, j int) bool {
	return ib[i].Time.String() < ib[j].Time.String()
}

func (ib Inbox) Swap(i, j int) {
	ib[i], ib[j] = ib[j], ib[i]
}

func GetSourceIp(item cache.Item) (ip string) {
	ip = item.Object.(*EmailCompose).SourceIP
	return
}

func GetFrom(item cache.Item) (from string) {
	from = item.Object.(*EmailCompose).From
	return
}

func GetUser(item cache.Item) (user string) {
	user = item.Object.(*EmailCompose).User
	return
}

func GetData(item cache.Item) (data string) {
	data = item.Object.(*EmailCompose).Data
	return
}

func GetBoundary(item cache.Item) (boundary string) {
	boundary = item.Object.(*EmailCompose).Boundary
	return
}

func Get(c *cache.Cache, m map[string][]string) (tmpI Inbox) {
	ip := m["ip"]
	from := m["from"]
	user := m["user"]

	for _, item := range c.Items() {
		filter := true
		if len(ip) != 0 {
			for _, i := range ip {
				if ok, _ := regexp.Match(i, []byte(GetSourceIp(item))); ok {
					break
				} else {
					filter = false
				}
			}
		}
		if len(from) != 0 && filter {
			for _, f := range from {
				if ok, _ := regexp.Match(f, []byte(GetFrom(item))); ok {
					filter = true
					break
				} else {
					filter = false
				}
			}
		}
		if len(user) != 0 && filter {
			for _, u := range user {
				if u == GetUser(item) {
					filter = true
					break
				} else {
					filter = false
				}
			}
		}
		if filter {
			tmpI = append(tmpI, item.Object.(*EmailCompose))
		}
	}
	sort.Sort(sort.Reverse(tmpI))
	return
}

func GetMessage(inboxCache *cache.Cache, id string) (text string, err error) {
	var body string
	if item, ok := inboxCache.Get(id); ok {
		msg, _ := mail.ReadMessage(strings.NewReader(item.(*EmailCompose).Data))

		if b, err := ioutil.ReadAll(msg.Body); err == nil {
			body = string(b)

			scanner := bufio.NewScanner(strings.NewReader(body))
			var ctFound int
			for scanner.Scan() {
				line := scanner.Text()
				if ok, _ := regexp.Match(`Content-Transfer-Encoding:`, []byte(line)); ok {
					ctFound += 1
				} else {
					if ctFound >= 1 {
						if ok, _ := regexp.Match("^--"+item.(*EmailCompose).Boundary, []byte(line)); ok {
							break
						} else {
							text += line
						}
					}
				}
			}
		}
	}
	return
}
