package session

import (
	"errors"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"sync"
	"time"
)

var SuperSession sessionMgr
const(
	SESSION_REDIS = iota
	SESSION_MEMORY
)

func Init(sessionType int,addr string, options ...interface{}){
	if sessionType == SESSION_MEMORY {
		SuperSession = NewmemorySessionMgr()
	}else if sessionType == SESSION_REDIS {
		SuperSession = NewRedisSessionMgr()
	}
	SuperSession.Init(addr,options...)
}

// session 管理
type sessionMgr interface {
	//初始化
	Init(addr string, options ...interface{})
	//创建session
	CreateSession(sessionId string) (session, error)
	//获取session
	GetSession(sessionId string) (session, error)
	//删除session
	DelSession(sessionId string) error
	//过期处理session
	TimeOutSession()
	//重置session最后操作时间
	ReSetLastUpdateTime(sessionId string)
}

type memorySessionMgr struct {
	sessionMap map[string]session //存放session的列表
	lock       sync.RWMutex
	expire     int64
}

func NewmemorySessionMgr() sessionMgr{
	return &memorySessionMgr{
		sessionMap: make(map[string]session,50),
	}
}

/**
options :
[0] 过期时间
*/
func (m *memorySessionMgr) Init(addr string, options ...interface{}) {
	if options != nil && len(options) > 0 {
		expire := options[0].(int) //过期时间
		m.expire = int64(expire)
	}
	//启动超时任务处理
	go m.TimeOutSession()
}

func (m *memorySessionMgr) CreateSession(sessionId string) (session, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	msession := NewMemorySession(sessionId)
	m.sessionMap[sessionId] = msession
	return msession, nil
}
func (m *memorySessionMgr) GetSession(sessionId string) (session, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	session, ok := m.sessionMap[sessionId]
	if !ok {
		return nil, errors.New("session is not create")
	}
	return session, nil
}
func (m *memorySessionMgr) DelSession(sessionId string) (err error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.sessionMap, sessionId)
	return
}

func (m *memorySessionMgr) TimeOutSession() {
	if len(m.sessionMap) < 1 { //没有session时,暂停一秒后.继续进行过期处理
		time.Sleep(time.Second)
	} else {
		for _, v := range m.sessionMap {
			//最大有效时间为 t
			t := (v.(*memorySession).lastUpdateTime.Unix()) + m.expire
			if t < time.Now().Unix() { //超时删除
				fmt.Printf("%#v expire , remove ... \n", v.(*memorySession).attributes)
				//delete(m.sessionMap, v.(*memorySession).sessionId)
				m.DelSession(v.(*memorySession).sessionId)
			}
		}
	}
	//运行后继续调用自己
	m.TimeOutSession()
}
func (m *memorySessionMgr)ReSetLastUpdateTime(sessionId string){
	if session,ok:=m.sessionMap[sessionId]; ok{
		session.(*memorySession).lastUpdateTime = time.Now()
	}
}


type redisSessionMgr struct {
	sessionMap map[string]session //存放session的列表
	pool       *redis.Pool
	lock       sync.RWMutex
	expire     int64
}

func NewRedisSessionMgr() sessionMgr{
	return &redisSessionMgr{
		sessionMap: make(map[string]session,50),
	}
}

/*
	options[0].(int64)    expire    :=   过期时间
	options[1].(string)   password  :=   数据库密码
	options[2].(int)      dbNum     :=   选择db数
	options[3].(int)      maxActive :=   最大连接数
	options[4].(int)      MaxIdle   :=   最大闲置连接数
 */
func (r *redisSessionMgr) Init(addr string, options ...interface{}) {
	expire := options[0].(int)
	//password := options[1].(string)
	dbNum := options[2].(int)
	maxActive := options[3].(int)
	MaxIdle := options[4].(int)
	
	pool := &redis.Pool{
		MaxActive:   maxActive,                        //100,                              //  最大连接数，即最多的tcp连接数，一般建议往大的配置，但不要超过操作系统文件句柄个数（centos下可以ulimit -n查看）
		MaxIdle:     MaxIdle,                          //100,                              // 最大空闲连接数，即会有这么多个连接提前等待着，但过了超时时间也会关闭。
		IdleTimeout: time.Duration(100) * time.Second, // 空闲连接超时时间，但应该设置比redis服务器超时时间短。否则服务端超时了，客户端保持着连接也没用
		Wait:        true,                             // 当超过最大连接数 是报错还是等待， true 等待 false 报错
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", addr, redis.DialDatabase(dbNum))
			if err != nil {
				return nil, fmt.Errorf("connect redis failed , err : %s", err)
			}
			//验证redis密码
			/*if _, authErr := conn.Do("AUTH", password); authErr != nil {
				return nil, fmt.Errorf("redis auth password error: %s", authErr)
			}*/
			return conn, nil
		},
	}
	r.expire = int64(expire)
	r.pool = pool
	//启动超时任务处理
	go r.TimeOutSession()
}

func (r *redisSessionMgr) CreateSession(sessionId string) (session, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	rsession := NewRedisSession(sessionId,r.pool)
	r.sessionMap[sessionId] = rsession
	return rsession, nil
}
func (r *redisSessionMgr) GetSession(sessionId string) (session, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	session, ok := r.sessionMap[sessionId]
	if !ok {
		return nil, errors.New("session is not create")
	}
	return session, nil
}
func (r *redisSessionMgr) DelSession(sessionId string) (err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if session,ok:= r.sessionMap[sessionId]; ok{
		//删除redis的用户信息
		session.(*redisSession).Del(sessionId)
		//删除内存的用户信息
		delete(r.sessionMap, sessionId)
	}
	return
}
func (r *redisSessionMgr) TimeOutSession() {
	fmt.Printf("start timeout check , data length : %d  data : %#v \n", len(r.sessionMap),r.sessionMap)
	if len(r.sessionMap) < 1 {
		time.Sleep(time.Second*3)
	}else{
		time.Sleep(time.Second*2)
		for _, v := range r.sessionMap {
			//最大有效时间为 t
			t := (v.(*redisSession).lastUpdateTime.Unix()) + r.expire
			fmt.Println(t,time.Now().Unix())
			if t < time.Now().Unix() { //超时删除
				fmt.Printf("%#v expire , remove ... \n", v.(*redisSession).attributes)
				//delete(r.sessionMap, v.(*redisSession).sessionId)
				r.DelSession(v.(*redisSession).sessionId)
			}
		}
	}
	r.TimeOutSession()
}
func (r *redisSessionMgr)ReSetLastUpdateTime(sessionId string){
	if session,ok:=r.sessionMap[sessionId]; ok{
		session.(*memorySession).lastUpdateTime = time.Now()
	}
}