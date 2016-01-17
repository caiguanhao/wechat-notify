package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const WECHAT_HOST = "https://api.weixin.qq.com/cgi-bin"
const TEMPLATEID = "u7WqGbcn5PBiFVFT6iba8ULsaRwYG2NKmulZ1NYvuEc"
const AUTO_URL_PREFIX = "https://dn-gaiamagic.qbox.me/auto-url.html"
const DESC_MAX_LENGTH = 200 // wechat's restriction

type Message interface {
	getType() string
}

type TextMessage struct {
	ToUser  string `json:"touser"`
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

func (_ TextMessage) getType() string {
	return "custom"
}

type TemplateMessage struct {
	ToUser     string      `json:"touser"`
	TemplateID string      `json:"template_id"`
	URL        string      `json:"url"`
	Data       interface{} `json:"data"`
}

func (_ TemplateMessage) getType() string {
	return "template"
}

type ValueColor struct {
	Value string `json:"value"`
	Color string `json:"color"`
}

type AccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type Input struct {
	Timestamp   int64
	Service     string
	Event       string
	Action      string
	Host        string
	Description string
	URL         string
}

type Response struct {
	Code    int    `json:"errcode"`
	Message string `json:"errmsg"`
}

func getAccessToken() (*AccessToken, error) {
	url := fmt.Sprintf("%s/token?grant_type=client_credential&appid=%s&secret=%s", WECHAT_HOST, APPID, SECRET)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var data AccessToken
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func send(msg Message) error {
	token, err := getAccessToken()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/message/%s/send?access_token=%s", WECHAT_HOST, msg.getType(), token.AccessToken)
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json; charset=utf-8", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var ret Response
	err = json.Unmarshal(body, &ret)
	if err != nil {
		return err
	}
	if ret.Code != 0 {
		return errors.New(ret.Message)
	}
	return nil
}

func parse(input []byte) *Input {
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimSpace(input)))
	var ret Input
	isDesc := false
	for scanner.Scan() {
		line := scanner.Text()
		if !isDesc && len(line) == 0 {
			isDesc = true
			continue
		}
		if isDesc {
			ret.Description = ret.Description + line + "\n"
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			ret.Description = ret.Description + line + "\n"
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "timestamp":
			ret.Timestamp, _ = strconv.ParseInt(value, 10, 64)
		case "service":
			ret.Service = value
		case "event":
			ret.Event = value
		case "action":
			ret.Action = value
		case "host":
			ret.Host = value
		case "url":
			ret.URL = value
		}
		ret.Description = ""
	}
	ret.Description = strings.TrimSpace(ret.Description)
	return &ret
}

// https://github.com/golang/crypto/blob/master/ssh/terminal/util.go
func isTerminal(fd int) bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), ioctlReadTermios, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}

func autoUrl(input string) (output string) {
	output = AUTO_URL_PREFIX + "#" + base64.StdEncoding.EncodeToString([]byte(input))
	return
}

var sendRaw bool
var noAutoUrl bool
var ioctlReadTermios uintptr

func init() {
	if runtime.GOOS == "darwin" {
		ioctlReadTermios = 0x40487413
	} else {
		ioctlReadTermios = 0x5401
	}

	flag.BoolVar(&sendRaw, "raw", false, "")
	flag.BoolVar(&noAutoUrl, "no-auto-url", false, "")
	flag.Usage = func() {
		fmt.Println("USAGE: wechat-notify [OPTION] [USER-OPENID] ...")
		fmt.Println()
		fmt.Println("Send templated message to specified WeChat users.")
		fmt.Println("For more info, visit https://github.com/caiguanhao/wechat-notify")
		fmt.Println()
		fmt.Println("Option:")
		fmt.Println("    --raw           send un-templated message")
		fmt.Println("    --no-auto-url   don't generate URL when URL is empty and message is too long")
		fmt.Println()
		fmt.Println("Template Format:")
		fmt.Println("    timestamp: 1452504535")
		fmt.Println("    service:   some-service")
		fmt.Println("    event:     some-event")
		fmt.Println("    action:    some-action")
		fmt.Println("    host:      some-host")
		fmt.Println()
		fmt.Println("    you can type your multi-line message here...")
	}
	flag.Parse()
}

func main() {
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Please provide at least one OPENID.")
		os.Exit(1)
	}
	if isTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "Paste your message and press CTRL-D to send. See --help for template format.")
	}
	stdin, stdinErr := ioutil.ReadAll(os.Stdin)
	if stdinErr != nil {
		fmt.Fprintln(os.Stderr, stdinErr)
		os.Exit(1)
	}
	var err error
	var errCount int
	for _, arg := range flag.Args() {
		parts := strings.SplitN(arg, "@", 2)
		openid := parts[0]
		if sendRaw {
			msg := TextMessage{}
			msg.ToUser = openid
			msg.MsgType = "text"
			msg.Text.Content = string(stdin)
			err = send(msg)
		} else {
			input := parse(stdin)
			datetime := ""
			if input.Timestamp > 0 {
				datetime = time.Unix(input.Timestamp, 0).Format("2006-01-02 15:04:05")
			}
			msg := TemplateMessage{}
			msg.ToUser = openid
			msg.TemplateID = TEMPLATEID
			msg.URL = input.URL
			description := input.Description
			infoLen := len(datetime) + len(input.Host) + len(input.Action)
			if len(description)+infoLen > DESC_MAX_LENGTH {
				if noAutoUrl == false && msg.URL == "" {
					msg.URL = autoUrl(description)
				}
				description = description[0:DESC_MAX_LENGTH-infoLen-3] + "..."
			}
			msg.Data = struct {
				Description ValueColor `json:"first"`
				DateTime    ValueColor `json:"time"`
				Host        ValueColor `json:"ip_list"`
				Type        ValueColor `json:"sec_type"`
				Remark      ValueColor `json:"remark"`
			}{
				Description: ValueColor{description, "#000"},
				DateTime:    ValueColor{datetime, "#000"},
				Host:        ValueColor{input.Host, "#000"},
				Type:        ValueColor{input.Action, "#000"},
			}
			err = send(msg)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, openid, err)
			errCount++
		} else {
			fmt.Println(openid, "ok")
		}
	}
	os.Exit(errCount)
}
