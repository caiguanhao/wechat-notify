package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const WECHAT_HOST = "https://api.weixin.qq.com/cgi-bin"
const TEMPLATEID = "u7WqGbcn5PBiFVFT6iba8ULsaRwYG2NKmulZ1NYvuEc"
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

func die(err interface{}) {
	log.Println(err)
	os.Exit(1)
}

var sendRaw bool

func init() {
	flag.BoolVar(&sendRaw, "raw", false, "")
	flag.Usage = func() {
		fmt.Println("USAGE: wechat-notify [--raw] [USER-OPENID] ...")
		fmt.Println()
		fmt.Println("This program will send templated message to specified users.")
		fmt.Println("Provide --raw to just send un-templated message, otherwise")
		fmt.Println("you need to provide template data like this via STDIN:")
		fmt.Println()
		fmt.Println("timestamp: 1452504535")
		fmt.Println("service:   some-service")
		fmt.Println("event:     some-event")
		fmt.Println("action:    some-action")
		fmt.Println("host:      some-host")
		fmt.Println("")
		fmt.Println("you can type your multi-line message here...")
		fmt.Println("")
		fmt.Println("For more info, visit https://github.com/caiguanhao/wechat-notify")
	}
	flag.Parse()
}

func main() {
	if flag.NArg() == 0 {
		die("Please provide at least one OPENID.")
	}
	stdin, stdinErr := ioutil.ReadAll(os.Stdin)
	if stdinErr != nil {
		die(stdinErr)
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
			log.Println(openid, err)
			errCount++
		} else {
			log.Println(openid, "ok")
		}
	}
	os.Exit(errCount)
}
