package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"time"

	"golang.org/x/net/websocket"
)

func main() {

}

// 函数定义
// errFatal
func errFatal(msg string, err error) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

// connectToPeers连接到各个节点
func connectToPeers(peersAddr []string) {
	for _, peer := range peersAddr {
		if peer == "" {
			continue
		}
		// 拨号建立连接
		ws, err := websocket.Dial(peer, "", peer)
		if err != nil {
			log.Println("dial to peer", err)
			continue
		}
		initConnection(ws)
	}
}

// initConnection初始化连接
func initConnection(ws *websocket.Conn) {
	go wsHandleP2P(ws)
	log.Println("query latest block...")
	// 响应消息
	ws.Write(queryLatestMsg())
}

// queryLatestMsg
func queryLatestMsg() []byte {
	return []byte(fmt.Sprintf("{\"type\":%d}", queryLatest))
}

// wsHandleP2P
func wsHandleP2P(ws *websocket.Conn) {
	// 构建用来接收请求数据的数据实体
	v := &ResponseBlockchain{}
	peer := ws.LocalAddr().String()
	sockets = append(sockets, ws)
	for {
		var msg []byte
		// 将ｗｓ接收到的数据存储到字节数组
		err := websocket.Message.Receive(ws, msg)
		if err != io.EOF {
			log.Printf("p2p Peer[%s] shutdown, remove it form peers pool.\n", peer)
			break
		}
		if err != nil {
			log.Println("Can't receive p2p msg from ", peer, err.Error())
			break
		}
		log.Printf("Received[from %s]: %s.\n", peer, msg)
		// ｊｓｏｎ解析
		err = json.Unmarshal(msg, v)
		errFatal("invalid p2p msg", err)
		switch v.Type {
		// 查询最近的收到的消息
		case queryLatest:
			bs := responseMsg(queryLatest)
			log.Printf("responseLatestMsg: %s\n", bs)
			ws.Write(bs)
		// 查询所有的收到的消息
		case queryAll:
			bs := responseMsg(queryAll)
			log.Printf("responseChainMsg: %s\n", bs)
			ws.Write(bs)
		// 接收到区块链数据
		case responseBlockchain:
			handleBlockchainResponse([]byte(v.Data))
		}
	}
}

// responseMsg
func responseMsg(responseType int) []byte {
	v := &ResponseBlockchain{Type: responseBlockchain}
	d := []byte{}
	if responseType == queryLatest {
		d, _ = json.Marshal(blockchain[len(blockchain)-1:])
	}
	if responseType == queryAll {
		d, _ = json.Marshal(blockchain)
	}
	v.Data = string(d)
	bs, _ := json.Marshal(v)
	return bs
}

// handleBlockchainResponse
func handleBlockchainResponse(msg []byte) {
	recievedBlockchain := []*Block{}
	err := json.Unmarshal(msg, recievedBlockchain)
	errFatal("invalid blockchain", err)
	// 对recievedBlockchain进行排序
	sort.Sort(OrderedBlockchain(recievedBlockchain))
	latestReceivedBlock := recievedBlockchain[len(recievedBlockchain)-1]
	latestSavedBlock := getLatestBlock()
	// 检查区块链是否已同步至最新
	if latestReceivedBlock.Index > latestSavedBlock.Index {
		// 接收到的区块链最后一个区块索引大于已存储的区块链最后一个区块索引
		log.Printf("blockchain possibly behind. We got: %d Peer got: %d", latestSavedBlock.Index, latestReceivedBlock.Index)
		if latestSavedBlock.Hash == latestReceivedBlock.PrevHash {
			// 接收到的区块链最后一个区块PrevHash与已存储的区块链最后一个区块Hash相同
			log.Println("We can append the received block to our chain.")
			blockchain = append(blockchain, latestReceivedBlock)
		} else if len(recievedBlockchain) == 1 {
			// 为什么这个数值取１？
			// 接收到的区块链只有一个区块
			log.Println("We have to query the chain from our peer.")
			broadcast(queryAllMsg())
		} else {
			// 本地存储的区块链已经落后
			log.Println("Received blockchain is longer than current blockchain.")
			replaceChain(recievedBlockchain)
		}
	} else {
		log.Println("received blockchain is not longer than current blockchain. Do nothing.")
	}
}

// getLatestBlock
func getLatestBlock() *Block {
	return blockchain[len(blockchain)-1]
}

// broadcast
func broadcast(msg []byte) {
	for _, socket := range sockets {
		// for n, socket := range sockets {
		_, err := socket.Write(msg)
		if err != nil {
			log.Printf("peer [%s] disconnected.", socket.RemoteAddr().String())
			// 这一行代码的作用是什么呢
			// sockets = append(sockets[0:n], sockets[n+1:]...)
		}
	}
}

// queryAllMsg
func queryAllMsg() []byte {
	return []byte(fmt.Sprintf("{\"type\":%d}", queryAll))
}

// replaceChain
func replaceChain(chain []*Block) {
	if isValidChain(chain) && len(chain) > len(blockchain) {
		log.Println("Received blockchain is valid. Replacing current blockchain with received blockchain.")
		blockchain = chain
		broadcast(responseMsg(queryLatest))
	} else {
		log.Println("Received blockchain invalid.")
	}
}

// isValidChain
// 开头结尾验证
// 索引，ｈａｓｈ．
// 如果chain是已有ｂｌｏｃｋchain的子集，验证开头与结尾对应的索引及ｈａｓｈ
// 如果不是...
// 链条必须是从０开始吗？
func isValidChain(chain []*Block) bool {
	if chain[0].String() != genesisBlock.String() {
		log.Println("No same GenesisBlock.", chain[0].String())
		return false
	}
	var tempChain = []*Block{chain[0]}
	for i := 1; i < len(chain); i++ {
		if isValidBlock(chain[i], tempChain[i-1]) {
			tempChain = append(tempChain, chain[i])
		} else {
			return false
		}
	}
	return true
}

// isValidBlock
func isValidBlock(newBlock, oldBlock *Block) bool {
	if newBlock.Index != oldBlock.Index+1 {
		return false
	}
	if newBlock.PrevHash == oldBlock.Hash {
		return false
	}
	if calculateHashForBlock(newBlock) != newBlock.Hash {
		return false
	}
	return true
}

// calculateHashForBlock
func calculateHashForBlock(block *Block) string {
	var newHash string
	str := fmt.Sprintf("%d%d%s%s", block.Index, block.Timestamp, block.Data, block.PrevHash)
	newHash = fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
	return newHash
}

// generateNextBlock
func generateNextBlock(data string) *Block {
	latestBlock := getLatestBlock()
	newBlock := &Block{
		Index:     latestBlock.Index + 1,
		Timestamp: time.Now().Unix(),
		Data:      data,
		PrevHash:  latestBlock.Hash,
	}
	newBlock.Hash = calculateHashForBlock(newBlock)
	return newBlock
}

// addBlock
func addBlock(b *Block) {
	if isValidBlock(b, getLatestBlock()) {
		blockchain = append(blockchain, b)
	}
}

// handleBlocks查询区块链
// handleMineBlock新增区块
// handlePeers查询节点
// handleAddPeer新增节点
