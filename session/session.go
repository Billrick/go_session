package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"sync"
	"time"
)

//session接口
type session interface {
	//初始化
	Init()
	//向内存放入值
	Set(key string, value interface{}) error
	//取内存中的值
	Get(key string) (value interface{}, err error)
}

//->内存sesssion
type memorySession struct {
	sessionId      string
	attributes     map[string]interface{}
	lock           sync.RWMutex
	lastUpdateTime time.Time
}

func NewMemorySession(sessionId string) session {
	return &memorySession{
		sessionId:      sessionId,
		attributes:     make(map[string]interface{}, 16),
		lastUpdateTime: time.Now(),
	}
}

func (m *memorySession) Init() {

}

func (m *memorySession) Set(key string, value interface{}) (err error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.attributes[key] = value
	return
}

func (m *memorySession) Get(key string) (value interface{}, err error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	v, ok := m.attributes[key]
	if !ok { //不存在
		return nil, errors.New(key + " attr not found")
	}
	return v, err
}

//->redissesssion
type redisSession struct {
	sessionId      string
	attributes     map[string]interface{}
	pool           *redis.Pool
	lock           sync.RWMutex
	lastUpdateTime time.Time
}

func NewRedisSession(sessionId string,pool *redis.Pool) session {
	fmt.Printf("session : %s create success !\n",sessionId)
	return &redisSession{
		sessionId:      sessionId,
		attributes:     make(map[string]interface{}, 16),
		lastUpdateTime: time.Now(),
		pool: pool,
	}
}

func (r *redisSession) Init() {

}

func (r *redisSession) Set(key string, value interface{}) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	//首先保持到内存中
	r.attributes[key] = value
	connect := r.pool.Get() //获取连接
	data, err := json.Marshal(r.attributes)
	if err != nil {
		return err
	}
	//向redis保存数据
	_, err = connect.Do("SET", r.sessionId, string(data))
	if err != nil {
		return err
	}
	return nil
}

func (r *redisSession) Get(key string) (value interface{}, err error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	//先从内存中取
	v, ok := r.attributes[key]
	/**
	ok = false 有两种情况
	1. 没有存储数据
	2. 分布式，可能不在本机的内存中。 从redis中取
	*/
	if !ok {
		connect := r.pool.Get()
		data, err := redis.Bytes(connect.Do("GET", r.sessionId))
		if err != nil {
			return nil, err
		}
		tmp := make(map[string]interface{}, 16)
		err = json.Unmarshal(data, &tmp)
		if err != nil {
			return nil, err
		}
		//把从redis取出的最新数据放入内存
		r.attributes = tmp
		//从最新的数据 根据key取数据
		value, ok := tmp[key]
		if !ok {
			return nil, errors.New(key + " attr not found")
		}
		return value, err
	} else {
		return v, err
	}
}


func (r *redisSession)Del(key string)(error){
	if _,err := r.pool.Get().Do("DEL",key) ; err != nil{
		return err
	}
	return nil
}