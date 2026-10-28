package vulnerable

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/tls"
	"net/http"
	"os"
)

func Bad() {
	_ = md5.New()
	_ = sha1.New()
	_ = &tls.Config{InsecureSkipVerify: true}
	_, _ = http.Get("http://example.internal/" + "x")
	_ = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345"
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	_ = "http://169.254.169.254/latest/meta-data"
}
