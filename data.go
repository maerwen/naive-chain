package main

import (
	"flag"
	"fmt"
	"time"

	"golang.org/x/net/websocket"
)

// 使用到的数据结构及常量，变量

//类型定义
// block	及其方法
type Block struct {
	Index     int64  `json:"index"`
	Timestamp int64  `json:"timestamp"`
	Data      string `json:"data"`
	PrevHash  string `json:"prevHash"`
	Hash      string `json:"hash"`
}

func (b *Block) String() string {
	return fmt.Sprintf("index: %d,previousHash:%s,timestamp:%d,data:%s,hash:%s", b.Index, b.PrevHash, b.Timestamp, b.Data, b.Hash)
}

// OrderedBlockchain	及实现一些方法,方便使用ｓｏｒｔ进行排序
// 直接把blockchain定义成struct之后，无法使用索引和预声明的len（）函数
type OrderedBlockchain []*Block

func (b OrderedBlockchain) Len() int {
	return len(b)
}
func (b OrderedBlockchain) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
func (b OrderedBlockchain) Less(i, j int) bool {
	return b[i].Index < b[j].Index
}

// Ｍｓｇ
// 接受各节点之间通信数据
type Msg struct {
	Type int    `json:"type"`
	Data string `json:"data"`
}

// 常量
// post请求请求实体中的一项数值，用以对请求进行区分相应
// wsHandleP2P方法中调用
const (
	// 查询最新区块
	QUERY_LATEST_BLOCK = iota
	// 查询整个链条
	QUERY_BLOCKCHAIN
	// 接收到广播消息，进行区块链更新
	UPDATE_BLOCKCHAIN
	// 添加一笔新的交易信息，生成新区块
	ADD_NEW_DATA
	// 查询所有节点
	QUERY_ALL_PEER
	// 新增节点
	ADD_NEW_PEER
)

// 全局变量
// 创世块
var genesisBlock *Block

func init() {
	genesisBlock = &Block{0, time.Now().Unix(), "my genesis block!!", "", ""}
	blockchain = append(blockchain, genesisBlock)
}

var (
	blockchain = []*Block{}
	// peer集合
	sockets []*websocket.Conn
	// 从命令行输入的参数
	httpAddr     = flag.String("api", ":3001", "api server address")
	p2pAddr      = flag.String("p2p", ":6001", "p2p server address")
	initialPeers = flag.String("peers", "ws://localhost:6001", "initial peers")
)
