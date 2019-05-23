package inbox

import (
	"bytes"
	"fmt"
	"github.com/patrickmn/go-cache"
	"testing"
)

func newEmail() (e EmailCompose) {
	// Use e.id default value: 00000000-0000-0000-0000-000000000000
	// Use e.Time default value: 0001-01-01 00:00:00 +0000 UTC
	e.From = "from@dump25.com"
	e.Rcpt = append(make([]string, 0), "rcpt@dump25.com", "rcpt2@dump25.com")
	e.Boundary = "----MIME delimiter"
	e.User = "test"
	e.SourceIP = "127.0.0.1:49891"
	e.Data =
		`From: "dump25Test" <from@dump25.com>
Reply-To: from@dump25.com
To: rcpt@dump25.com
Message-ID: <000000000000>
Subject: dump25 Test
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="----MIME delimiter"

------MIME delimiter
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

Confirmaci=C3=B3n del env=C3=ADo
___________________________________________________________________________

------MIME delimiter
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<html xmlns=3D"http://www.w3.org/1999/xhtml">
<head>=20
</head> =20
<body>
Para m=C3=A1s informaci=C3=B3n=

</body>
</html>`
	return
}

func TestGetMessage(t *testing.T) {
	e := newEmail()
	p := `
Confirmación del envío
___________________________________________________________________________


<html xmlns="http://www.w3.org/1999/xhtml">
<head>
</head>
<body>
Para más información
</body>
</html>`
	c := cache.New(0, 0)
	c.Set(e.Id.String(), &e, 0)
	s, _ := GetMessage(c, e.Id.String())
	if bytes.Equal([]byte(s), []byte(p)) {
		t.Errorf("\n- Got -\n%v\n- Expected -\n%v", s, p)
	}

}

func BenchmarkGetMessage(b *testing.B) {
	e := newEmail()
	c := cache.New(0, 0)
	c.Set(e.Id.String(), &e, 0)
	fmt.Println(GetMessage(c, e.Id.String()))
}

func TestGetId(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	id := GetId(i)

	if id != e.Id {
		t.Errorf("\n- Got -\n%v\n- Expected -\n%v", id, e.Id)
	}
}

func TestGetTime(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	tm := GetTime(i)
	if !e.Time.Equal(tm) {
		t.Errorf("Got %v, expected %v.", t, e.Time)
	}
}

func TestGetSourceIp(t *testing.T) {
	e := newEmail()
	c := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	sourceIP := GetSourceIp(c)
	if sourceIP != e.SourceIP {
		t.Errorf("Got %v, expected %v.", sourceIP, e.SourceIP)
	}
}

func TestGetUser(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	u := GetUser(i)
	if u != e.User {
		t.Errorf("Got %v, expected %v.", u, e.User)
	}
}

func TestGetFrom(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	f := GetFrom(i)
	if f != e.From {
		t.Errorf("Got %v, expected %v.", f, e.From)
	}
}

func TestGetRcp(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	r := GetRcp(i)
	if len(r) != len(e.Rcpt) {
		t.Errorf("Got %v, expected %v", len(r), len(e.Rcpt))
	}
}

func TestGetBoundary(t *testing.T) {
	e := newEmail()
	i := cache.Item{
		Expiration: 0,
		Object:     &e,
	}
	b := GetBoundary(i)
	if b != e.Boundary {
		t.Errorf("Got %v, expected %v.", b, e.Boundary)
	}
}
