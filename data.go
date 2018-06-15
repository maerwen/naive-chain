package main

import "fmt"

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

// blockchain	及实现一些方法,方便使用ｓｏｒｔ进行排序
type Blockchain []*Block

func (b Blockchain) Len() int {
	return len(b)
}
func (b Blockchain) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
func (b Blockchain) Less(i, j int) bool {
	return b[i].Index < b[j].Index
}

// ResponseBlockchain

const (
	queryLatest = iota
	queryAll
	responseBlockchain
)

// 创世块
