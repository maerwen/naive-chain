package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

func main() {
	// 读取命令行参数
	flag.Parse()
	connectToPeers(strings.Split(*initialPeers, ","))
	// 处理本地事务
	http.HandleFunc("/", homeHandle)
	// 为什么放到ｇｏ协程中去？
	go func() {
		log.Println("Listen HTTP on", *httpAddr)
		errFatal("start api server", http.ListenAndServe(*httpAddr, nil))
	}()
	// 处理ｐ２ｐ事务
	http.Handle("/p2p/", websocket.Handler(wsHandleP2P))
	log.Println("Listen P2P on ", *p2pAddr)
	errFatal("start p2p server", http.ListenAndServe(*p2pAddr, nil))
}
func homeHandle(w http.ResponseWriter, r *http.Request) {
	uri := r.RequestURI
	if uri == "/favicon.ico" {
		return
	}
	if r.Method == "GET" {
		handleBlocks(w, r)
	} else {
		msg := parseRequestBody(r)
		switch msg.Type {
		case ADD_NEW_DATA: //3
			handleMineBlock(msg.Data)
			fallthrough
		case QUERY_LATEST_BLOCK: //0
			queryLatestBlock(w)
		case ADD_NEW_PEER: //5
			handleAddPeer(msg.Data)
			fallthrough
		case QUERY_ALL_PEER: //4
			handlePeers(w)
		default:
		}
	}
}

// 请求实体数据读取
func parseRequestBody(r *http.Request) *Msg {
	msg := &Msg{}
	buff := []byte{}
	//为什么以下两种方式无法读取出来数据,报错unexpected end of JSON input
	// dec := json.NewDecoder(r.Body)
	// err := dec.Decode(&msg)

	// n, err := r.Body.Read(buff)

	buff, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	errFatal("read request body failed:", err)
	// fmt.Println(len(buff))
	err = json.Unmarshal(buff, msg)
	errFatal("json unmarshal msg failed:", err)
	return msg
}

// 函数定义
// errFatal
func errFatal(msg string, err error) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

// connectToPeers连接到各个节点
func connectToPeers(newPeersAddr []string) {
	for _, peer := range newPeersAddr {
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
	ws.Write(queryLatestTrade())
}

// queryLatestTrade
// 查询最新的区块所记录的交易
func queryLatestTrade() []byte {
	return []byte(fmt.Sprintf("{\"type\":%d}", QUERY_LATEST_BLOCK))
}

// wsHandleP2P
func wsHandleP2P(ws *websocket.Conn) {
	// 构建用来接收请求数据的数据实体
	v := &Msg{}
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
		case QUERY_LATEST_BLOCK:
			bs := responseMsg(QUERY_LATEST_BLOCK)
			log.Printf("responseLatestMsg: %s\n", bs)
			ws.Write(bs)
		// 查询所有的收到的消息
		case QUERY_BLOCKCHAIN:
			bs := responseMsg(QUERY_BLOCKCHAIN)
			log.Printf("responseChainMsg: %s\n", bs)
			ws.Write(bs)
		// 接收到有区块链数据更新的通知
		case UPDATE_BLOCKCHAIN:
			handleBlockchainResponse([]byte(v.Data))
		}
	}
}

// responseMsg
func responseMsg(responseType int) []byte {
	v := &Msg{Type: UPDATE_BLOCKCHAIN}
	d := []byte{}
	if responseType == QUERY_LATEST_BLOCK {
		d, _ = json.Marshal(blockchain[len(blockchain)-1:])
	}
	if responseType == QUERY_BLOCKCHAIN {
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
			broadcast(queryBlockchain())
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
func queryLatestBlock(w http.ResponseWriter) {
	buff, err := json.Marshal(getLatestBlock())
	if err != nil {
		http.Error(w, "json marshal bolck failed:"+err.Error(), 500)
	}
	w.Write(buff)
}

// broadcast
// 发送广播，向其他节点传递消息
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

// queryBlockchain
func queryBlockchain() []byte {
	return []byte(fmt.Sprintf("{\"type\":%d}", QUERY_BLOCKCHAIN))
}

// replaceChain
func replaceChain(chain []*Block) {
	if isValidChain(chain) && len(chain) > len(blockchain) {
		log.Println("Received blockchain is valid. Replacing current blockchain with received blockchain.")
		blockchain = chain
		broadcast(responseMsg(QUERY_LATEST_BLOCK))
	} else {
		log.Println("Received blockchain invalid.")
	}
}

// isValidChain
// 每个节点本地存储的链条都必须是从创世块开始！！！
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
// 判断当前区块是否有效
func isValidBlock(newBlock, oldBlock *Block) bool {
	if newBlock.Index != oldBlock.Index+1 {
		return false
	}
	if newBlock.PrevHash != oldBlock.Hash {
		return false
	}
	if calculateHashForBlock(newBlock) != newBlock.Hash {
		return false
	}
	return true
}

// calculateHashForBlock
// 根据已有参数，计算出新区块的ＨＡＳＨ值
func calculateHashForBlock(block *Block) string {
	var newHash string
	str := fmt.Sprintf("%d%d%s%s", block.Index, block.Timestamp, block.Data, block.PrevHash)
	newHash = fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
	return newHash
}

// handleBlocks查询区块链，以ｊｓｏｎ格式发送给浏览器
func handleBlocks(w http.ResponseWriter, r *http.Request) {
	bc, _ := json.MarshalIndent(blockchain, "", " ")
	w.Write(bc)
}

// handleMineBlock新增区块
// 根据浏览器传入的信息生成一个新区块
func handleMineBlock(data string) {
	block := generateNewBlock(data)
	addBlockToBlockchain(block)
	broadcast(responseMsg(QUERY_LATEST_BLOCK))
}

// addBlockToBlockchain
// 将当前区块新增到本地区块链上
func addBlockToBlockchain(b *Block) {
	if isValidBlock(b, getLatestBlock()) {
		blockchain = append(blockchain, b)
	}
}

// generateNewBlock
// 根据下一条交易信息生成新的区块
func generateNewBlock(data string) *Block {
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

// handlePeers查询节点
func handlePeers(w http.ResponseWriter) {
	var peers []string
	for _, socket := range sockets {
		if socket.IsClientConn() {
			peers = append(peers, socket.LocalAddr().String())
		} else {
			peers = append(peers, socket.Request().RemoteAddr)
		}
	}
	buff, _ := json.Marshal(peers)
	w.Write(buff)
}

// handleAddPeer新增节点
func handleAddPeer(data string) {
	connectToPeers([]string{data})
}
