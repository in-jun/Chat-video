package main

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed HTML/index.html
var indexPage string

type user struct {
	ws    *websocket.Conn
	ip    string
	check bool
}

type Room struct {
	user1 user
	user2 user
}

const max = 100

var clients [max]Room

var (
	upgrader = websocket.Upgrader{}
)

func main() {
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/ws", handleWebSocket)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("서버 시작 오류: %v", err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", indexPage)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	socket, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("오류: WebSocket 연결 업그레이드에 실패했습니다: %v", err)
		return
	}
	ip := getClientPublicIP(r)
	log.Printf("WebSocket 연결이 %s에서 성공적으로 수립되었습니다", ip)
	RoomMatching(socket, ip)
}

func RoomMatching(ws *websocket.Conn, ip string) {
	log.Printf("%s에서 온 WebSocket 연결을 위한 방을 찾는 중입니다", ip)

	for {
		for i := 0; i < max; i++ {
			if clients[i].user1.check {
				if !clients[i].user2.check {
					clients[i].user2.check = true
					clients[i].user2.ws = ws
					clients[i].user2.ip = ip
					log.Printf("WebSocket 연결이 방 %d의 User 2에 할당되었습니다: %s", i, ip)
					clients[i].MessageHandler()
					return
				}
			}
		}
		for i := 0; i < max; i++ {
			if !clients[i].user1.check {
				clients[i].user1.check = true
				clients[i].user1.ws = ws
				clients[i].user1.ip = ip
				log.Printf("WebSocket 연결이 방 %d의 User 1에 할당되었습니다: %s", i, ip)
				return
			}
		}
		log.Printf("모든 방이 꽉 찼습니다. 대기 중입니다...")
		time.Sleep(time.Second)
	}
}

func (room *Room) MessageHandler() {
	defer room.reset()

	user1Done := make(chan struct{})
	user2Done := make(chan struct{})

	handleMessage := func(sender *user, receiver *user, done chan struct{}) {
		for {
			messageType, msg, err := sender.ws.ReadMessage()
			if err != nil || receiver.ws.WriteMessage(messageType, msg) != nil {
				done <- struct{}{}
				log.Printf("오류: %s와 %s 간의 메시지 교환 중 오류 발생", sender.ip, receiver.ip)
				return
			}
		}
	}

	go handleMessage(&room.user1, &room.user2, user1Done)
	go handleMessage(&room.user2, &room.user1, user2Done)

	select {
	case <-user1Done:
		log.Printf("User 1 %s가 연결을 끊었습니다", room.user1.ip)
	case <-user2Done:
		log.Printf("User 2 %s가 연결을 끊었습니다", room.user2.ip)
	case <-time.After(time.Second * 3):
		log.Printf("User 1 %s와 User 2 %s 간의 메시지 교환 시간 초과", room.user1.ip, room.user2.ip)
	}
}

func (room *Room) reset() {
	log.Printf("User 1 %s와 User 2 %s 간의 방 초기화 중", room.user1.ip, room.user2.ip)

	room.user1.check = false
	room.user2.check = false

	err1 := room.user1.ws.Close()
	if err1 != nil {
		log.Printf("오류: User 1 %s의 연결을 닫을 수 없습니다: %v", room.user1.ip, err1)
	}
	err2 := room.user2.ws.Close()
	if err2 != nil {
		log.Printf("오류: User 2 %s의 연결을 닫을 수 없습니다: %v", room.user2.ip, err2)
	}

	log.Printf("User 1 %s와 User 2 %s 간의 방 초기화 완료", room.user1.ip, room.user2.ip)
}

func getClientPublicIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
}
