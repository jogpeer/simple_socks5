package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var up, down int64
var count int64
var speedUP, speedDown int64

type Flag int32

const (
	Traffic_Up   Flag = 1
	Traffic_Down Flag = 2
)

type Config struct {
	Port          int    `json:"port"`
	IsAuth        bool   `json:"isAuth"`
	UserName      string `json:"userName"`
	UserPwd       string `json:"userPwd"`
	StatusWebPort int    `json:"statusWebPort"`
}

type Status struct {
	AuthStatus string
	Incoming   int64
	Outgoing   int64
	Connect    int64
	SpeedUP    int64
	SpeedDown  int64
}

var cfg Config

func main() {
	cfgPath := "config.json"
	if !checkFileIsExist(cfgPath) {
		fmt.Println("Config file not find!")
		return
	}
	content, err := ioutil.ReadFile(cfgPath)
	if err != nil {
		fmt.Println("Read config file error")
		return
	}
	err1 := json.Unmarshal(content, &cfg)
	if err1 != nil {
		fmt.Println("config file parse err")
		return
	}
	listen, err2 := net.Listen("tcp", ":"+strconv.Itoa(cfg.Port))
	if err2 != nil {
		fmt.Println("listen err")
		return
	}
	go func() {
		for {
			conn, err := listen.Accept()
			if err != nil {
				fmt.Println("accept err")
				continue
			}
			go dealTcpStream(conn)
		}
	}()

	go countSpeed()
	go http.HandleFunc("/status", socksServerStatus)
	http.ListenAndServe(":"+strconv.Itoa(cfg.StatusWebPort), nil)
}

func socksServerStatus(w http.ResponseWriter, r *http.Request) {
	var status Status
	status.AuthStatus = strconv.FormatBool(cfg.IsAuth)
	status.Connect = count
	status.Incoming = down
	status.Outgoing = up
	status.SpeedDown = speedDown
	status.SpeedUP = speedUP
	data, _ := json.Marshal(status)
	w.Header().Set("content-type", "application/json")
	w.Write(data)

}

func countSpeed() {
	for {
		up1 := up
		down1 := down
		time.Sleep(time.Duration(1) * time.Second)
		speedUP = (up - up1)
		speedDown = (down - down1)
	}
}

func dealTcpStream(conn net.Conn) {
	buffer := make([]byte, 1024)
	n, _ := conn.Read(buffer)
	if n < 3 {
		conn.Close()
		fmt.Println("protol err")
		return
	}

	if cfg.IsAuth { //是否需要认证
		buffer[1] = 0x02 //直接账号和密码认证
		conn.Write(buffer[0:2])
		_, aerr := conn.Read(buffer)
		if aerr != nil {
			conn.Close()
			fmt.Println("Get auth content err!")
			return
		}
		fmt.Println("Auth version : ", buffer[0])
		usernameLen := int(buffer[1])
		user := string(buffer[2 : usernameLen+2])
		pwdlength := int(buffer[2+usernameLen])
		userpwd := string(buffer[usernameLen+3 : usernameLen+3+pwdlength])
		if user != cfg.UserName || userpwd != cfg.UserPwd { //认证失败
			buffer[1] = 0x01
			conn.Write(buffer[0:2])
			conn.Close()
			return
		} else {
			buffer[1] = 0x00
			conn.Write(buffer[0:2])
		}
	} else {
		buffer[1] = 0
		conn.Write(buffer[0:2])
	}
	count++
	n2, _ := conn.Read(buffer)
	var host, port string
	switch buffer[3] {
	case 0x01: //IP V4
		host = net.IPv4(buffer[4], buffer[5], buffer[6], buffer[7]).String()
	case 0x03: //域名
		host = string(buffer[5 : n2-2]) //b[4]表示域名的长度
	case 0x04: //IP V6
		host = net.IP{buffer[4], buffer[5], buffer[6], buffer[7], buffer[8], buffer[9], buffer[10], buffer[11], buffer[12], buffer[13], buffer[14], buffer[15], buffer[16], buffer[17], buffer[18], buffer[19]}.String()
	}
	port = strconv.Itoa(int(buffer[n2-2])<<8 | int(buffer[n2-1]))
	remotecon, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		fmt.Println("conn remote server err", err)
		return
	}
	buffer[0] = 0x05
	buffer[1] = 0x00
	buffer[2] = 0x00
	buffer[3] = 0x01
	buffer[4] = 0x00
	buffer[5] = 0x00
	buffer[6] = 0x00
	buffer[7] = 0x00
	buffer[8] = 0x00
	buffer[9] = 0x00
	conn.Write(buffer[0:10])
	go proxy(conn, remotecon, Traffic_Up)
	proxy(remotecon, conn, Traffic_Down)
	var mutex sync.Mutex
	mutex.Lock()
	count--
	mutex.Unlock()
}

func proxy(read net.Conn, write net.Conn, f Flag) {
	var mutex sync.Mutex
	defer func() {
		read.Close()
		write.Close()
	}()
	for {
		buf := make([]byte, 4096)
		n, err := read.Read(buf[0:])
		if err != nil {
			fmt.Println(err)
			return
		}
		if n > 0 {
			n1, err := write.Write(buf[0:n])
			if err != nil {
				fmt.Println(write.RemoteAddr(), "write:", n1, err)
				return
			}
		}
		if f == Traffic_Up {
			mutex.Lock()
			up += int64(n)
			mutex.Unlock()
		}
		if f == Traffic_Down {
			mutex.Lock()
			down += int64(n)
			mutex.Unlock()
		}
	}
}

func checkFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}
