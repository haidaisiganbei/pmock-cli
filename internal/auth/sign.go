// Package auth implements the PMOCK open API request signature.
//
// The scheme is documented in the integration guide:
//
//	timestamp    = current unix seconds floored to a whole minute
//	stringToSign = accessKey + ":" + timestamp
//	token        = base64(hmac-sha256(secretKey, stringToSign))
//
// The timestamp is not transmitted separately: the server derives it from the
// token, which is why a signature is reusable only within the same minute.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"time"
)

// Timestamp floors t to the minute boundary used by the signature.
func Timestamp(t time.Time) int64 {
	unix := t.Unix()
	return unix - unix%60
}

// Sign returns the timestamp and base64 signature for the given credentials.
func Sign(accessKey, secretKey string, t time.Time) (int64, string) {
	ts := Timestamp(t)
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(accessKey + ":" + strconv.FormatInt(ts, 10)))
	return ts, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
