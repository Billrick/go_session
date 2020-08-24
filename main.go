package main

import (
	"fmt"
	"github.com/google/uuid"
	"sync"
	"z.cn/20200823-session/session"
)
var wg sync.WaitGroup
func main(){
	session.Init(session.SESSION_REDIS,"127.0.0.1:6379",10,"",0,50,15)
	session,_ := session.SuperSession.CreateSession(uuid.New().String())
	err := session.Set("username","zhangsan")
	if err != nil {
		fmt.Println(err)
		return
	}
	wg.Add(1)
	wg.Wait()
}