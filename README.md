# dump25

## Intro

Thought to work along with your application, so that, you can run your own tests without the need of a real SMTP server, 
or impacting your customers with a fake email by accident.

Dump25 will dump a JSON with the current _Fake Email Queue_ at the root of its HTTP server.

The queue is stored in a Cache file (with a default expiration time), 
so the previous queue will be available after restarts, 
except items in queue that reached its expiration time.

## Command line parameters

Just a little bit of optional configuration (`./dump25 -help` output):
```
Usage of ./dump25:
  -expTime int
        Expiration time (hours) of each Item in Queue. (default 8)
  -httpPort string
        What port should the HTTP Server use. (default "10080")
  -smtpAuth
        Whatever if dump25 should ask for SMTP authentication.
  -smtpPort string
        What port should the fake SMTP Server use. (default "10025")
```

Note that enabling SMTP authentication, requires the client to provide user and password (anything), 
if authentication is not provided, then the client will be rejected, with the corresponding SMTP Error Code.

## Cache flush
Flushing the current Cache (or purging the queue):
```
http://localhost:10080/flush
```

## Filtering

It allows filtering by partial or exact match of IP and / or From address:
```
http://localhost:10080/?ip=127.0.0&from=test@dump25.com
``` 
If SMTP Authentication is or was enabled, you can also filter by the exact `UserName` used 
during authentication.
