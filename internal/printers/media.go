package printers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

func (s *Service) sign(printerID, name string, exp int64) string {
	mac := hmac.New(sha256.New, s.cfg.SignKey)
	fmt.Fprintf(mac, "printer-media:%s/%s/%d", printerID, name, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) mediaURL(printerID, name string) string {
	exp := time.Now().Add(mediaURLTTL).Truncate(mediaURLStep).Unix()
	return fmt.Sprintf("/domotics/printers/%s/recordings/%s?exp=%d&sig=%s", printerID, name, exp, s.sign(printerID, name, exp))
}

func (s *Service) verify(printerID, name, expRaw, sig string) bool {
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(s.sign(printerID, name, exp)))
}
