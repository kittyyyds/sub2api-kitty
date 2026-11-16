package service

import (
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

// Batch import of VM2-managed accounts. One line per account:
//
//	email---password---2fa-secret
//	email---password---2fa-secret---socks5://user:pass@host:port
//
// The email is everything before the first separator and the 2FA secret (and
// optional proxy) are taken from the end, so a password that itself contains
// the separator stays intact.

const (
	// OpenAIVMBatchMaxLines bounds one batch. Each account is its own slot; a
	// KVM slot reserves 1 GB of memory and can grow to 20 GB of disk.
	OpenAIVMBatchMaxLines = 20
	// OpenAIVMBatchMaxInputBytes bounds the pasted text before it is parsed.
	OpenAIVMBatchMaxInputBytes = 64 << 10

	openAIVMBatchSeparator = "---"
)

// OpenAIVMBatchLine is one parsed line. Password and MFASecret exist only in
// memory, for as long as it takes to send the login to the slot.
type OpenAIVMBatchLine struct {
	LineNo    int
	Name      string
	Email     string
	Password  string
	MFASecret string
	// ProxyURL is the proxy written on the line itself, if any.
	ProxyURL string
}

// OpenAIVMBatchLineError says why a line cannot be imported. It never quotes
// the line: the line holds a password.
type OpenAIVMBatchLineError struct {
	LineNo int    `json:"line_no"`
	Email  string `json:"email,omitempty"`
	Code   string `json:"code"`
}

// ParseOpenAIVMBatch splits the pasted text into importable lines and errors.
// Blank lines are skipped; line numbers are those of the pasted text.
func ParseOpenAIVMBatch(text string) ([]OpenAIVMBatchLine, []OpenAIVMBatchLineError) {
	var lines []OpenAIVMBatchLine
	var problems []OpenAIVMBatchLineError
	seen := map[string]int{}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for index, raw := range strings.Split(text, "\n") {
		lineNo := index + 1
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		line, code := parseOpenAIVMBatchLine(raw)
		line.LineNo = lineNo
		if code == "" {
			key := strings.ToLower(line.Email)
			if _, duplicate := seen[key]; duplicate {
				code = "duplicate_in_batch"
			} else {
				seen[key] = lineNo
			}
		}
		if code != "" {
			problems = append(problems, OpenAIVMBatchLineError{LineNo: lineNo, Email: openAIVMBatchSafeEmail(line.Email), Code: code})
			continue
		}
		lines = append(lines, line)
	}
	return lines, problems
}

func parseOpenAIVMBatchLine(raw string) (OpenAIVMBatchLine, string) {
	var line OpenAIVMBatchLine
	separator := openAIVMBatchSeparator
	first := strings.Index(raw, separator)
	if first >= 0 && strings.HasPrefix(raw[first:], "----") {
		separator = "----"
	}
	if first <= 0 {
		return line, "invalid_format"
	}
	line.Email = strings.TrimSpace(raw[:first])
	rest := raw[first+len(separator):]
	// An optional proxy is the last field and is recognisable by its scheme.
	if last := strings.LastIndex(rest, separator); last >= 0 {
		if tail := strings.TrimSpace(rest[last+len(separator):]); strings.Contains(tail, "://") {
			line.ProxyURL = tail
			rest = rest[:last]
		}
	}
	last := strings.LastIndex(rest, separator)
	if last < 0 {
		return line, "invalid_format"
	}
	line.Password = rest[:last]
	line.MFASecret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(rest[last+len(separator):]), " ", ""))
	switch {
	case !validOpenAIVMBatchEmail(line.Email):
		return line, "invalid_email"
	case strings.TrimSpace(line.Password) == "":
		return line, "missing_password"
	case !validOpenAIVMBatchMFASecret(line.MFASecret):
		// Without a 2FA secret a login stops at the code prompt and nobody is
		// there to answer it in a batch; such accounts are added one by one.
		return line, "invalid_mfa_secret"
	}
	if line.ProxyURL != "" {
		if _, ok := ParseOpenAIVMBatchProxy(line.ProxyURL); !ok {
			return line, "invalid_proxy"
		}
	}
	return line, ""
}

func validOpenAIVMBatchEmail(value string) bool {
	if value == "" || len(value) > 254 || strings.ContainsAny(value, " \t<>\"") {
		return false
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return false
	}
	at := strings.LastIndex(value, "@")
	return at > 0 && strings.Contains(value[at+1:], ".")
}

// A TOTP secret is base32: letters A-Z and digits 2-7, optional padding.
func validOpenAIVMBatchMFASecret(value string) bool {
	value = strings.TrimRight(value, "=")
	if len(value) < 16 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z') && !(r >= '2' && r <= '7') {
			return false
		}
	}
	return true
}

// openAIVMBatchSafeEmail keeps an email only when it looks like one, so a
// misplaced password is never echoed back as the "email" of a bad line.
func openAIVMBatchSafeEmail(value string) string {
	if validOpenAIVMBatchEmail(value) {
		return value
	}
	return ""
}

// OpenAIVMBatchProxy is a proxy written on a batch line.
type OpenAIVMBatchProxy struct {
	Protocol string
	Host     string
	Port     int
	Username string
	Password string
}

// ParseOpenAIVMBatchProxy accepts only what a slot can use: socks5 / socks5h
// with an explicit host and port.
func ParseOpenAIVMBatchProxy(raw string) (OpenAIVMBatchProxy, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "socks5" && u.Scheme != "socks5h") || u.Hostname() == "" {
		return OpenAIVMBatchProxy{}, false
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return OpenAIVMBatchProxy{}, false
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return OpenAIVMBatchProxy{}, false
	}
	proxy := OpenAIVMBatchProxy{Protocol: u.Scheme, Host: strings.ToLower(u.Hostname()), Port: port}
	if u.User != nil {
		proxy.Username = u.User.Username()
		proxy.Password, _ = u.User.Password()
	}
	return proxy, true
}

// AssignOpenAIVMBatchProxies gives every line without its own proxy one of the
// selected proxies, filling each up to perProxy accounts in the order given.
// `used` is how many accounts each proxy already carries. Lines that cannot be
// placed are returned in `unplaced` and are not created: an account is never
// started on a proxy nobody chose for it.
func AssignOpenAIVMBatchProxies(lines []OpenAIVMBatchLine, proxyIDs []int64, used map[int64]int, perProxy int) (assigned map[int]int64, unplaced []int) {
	assigned = map[int]int64{}
	if perProxy < 1 {
		perProxy = 1
	}
	load := map[int64]int{}
	for id, count := range used {
		load[id] = count
	}
	next := 0
	for _, line := range lines {
		if line.ProxyURL != "" {
			continue
		}
		placed := false
		for tries := 0; tries < len(proxyIDs); tries++ {
			id := proxyIDs[(next+tries)%len(proxyIDs)]
			if load[id] < perProxy {
				assigned[line.LineNo] = id
				load[id]++
				// Round-robin: the next line starts at the following proxy, so
				// accounts spread out before any proxy is filled.
				next = (next + tries + 1) % len(proxyIDs)
				placed = true
				break
			}
		}
		if !placed {
			unplaced = append(unplaced, line.LineNo)
		}
	}
	return assigned, unplaced
}
