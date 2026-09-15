package main

import (
  "bytes"
  "context"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "io"
  "log"
  "net/http"
  "os"
  "strconv"
  "strings"
  "time"
)

type RPC struct { URL string; Client *http.Client }
type rpcResp struct { Result json.RawMessage `json:"result"`; Error *struct{Code int `json:"code"`; Message string `json:"message"`} `json:"error"` }
type Block struct { Number string `json:"number"`; Transactions []Tx `json:"transactions"` }
type Tx struct { Hash string `json:"hash"`; To *string `json:"to"`; From string `json:"from"` }
type Receipt struct { ContractAddress *string `json:"contractAddress"`; Status string `json:"status"` }

func (r *RPC) call(ctx context.Context, method string, params any, out any) error {
  body,_:=json.Marshal(map[string]any{"jsonrpc":"2.0","id":1,"method":method,"params":params})
  req,err:=http.NewRequestWithContext(ctx,"POST",r.URL,bytes.NewReader(body)); if err!=nil{return err}; req.Header.Set("Content-Type","application/json")
  resp,err:=r.Client.Do(req); if err!=nil{return err}; defer resp.Body.Close(); b,_:=io.ReadAll(resp.Body); if resp.StatusCode/100!=2{return fmt.Errorf("rpc http %d: %s",resp.StatusCode,string(b))}
  var rr rpcResp; if err=json.Unmarshal(b,&rr); err!=nil{return err}; if rr.Error!=nil{return fmt.Errorf("rpc %d: %s",rr.Error.Code,rr.Error.Message)}; return json.Unmarshal(rr.Result,out)
}
func hexUint(s string)(uint64,error){return strconv.ParseUint(strings.TrimPrefix(s,"0x"),16,64)}
func envInt(k string,d int)int{v,_:=strconv.Atoi(os.Getenv(k));if v<=0{return d};return v}

func decodeABIString(raw string) string {
  b,err:=hex.DecodeString(strings.TrimPrefix(raw,"0x")); if err!=nil||len(b)==0{return ""}
  // dynamic ABI string: offset(32), length(32), data
  if len(b)>=64 { n:=int(newBig(b[32:64])); if n>0 && 64+n<=len(b){return strings.TrimSpace(string(b[64:64+n]))} }
  // bytes32 fallback
  if len(b)>=32{return strings.TrimSpace(strings.TrimRight(string(b[:32]),"\x00"))}; return ""
}
func newBig(b []byte) uint64 { var n uint64; start:=0;if len(b)>8{start=len(b)-8};for _,x:=range b[start:]{n=n<<8|uint64(x)};return n }
func tokenMeta(ctx context.Context,r *RPC,addr string)(string,string){
  call:=func(sig string)string{var out string; _=r.call(ctx,"eth_call",[]any{map[string]string{"to":addr,"data":sig},"latest"},&out);return decodeABIString(out)}
  return call("0x06fdde03"),call("0x95d89b41")
}
func telegram(ctx context.Context, token, chatID, text string) error {
  if token==""||chatID==""{log.Printf("TELEGRAM disabled: %s",text);return nil}
  u:="https://api.telegram.org/bot"+token+"/sendMessage"; payload,_:=json.Marshal(map[string]any{"chat_id":chatID,"text":text,"disable_web_page_preview":true})
  req,_:=http.NewRequestWithContext(ctx,"POST",u,bytes.NewReader(payload));req.Header.Set("Content-Type","application/json");resp,err:=http.DefaultClient.Do(req);if err!=nil{return err};defer resp.Body.Close();if resp.StatusCode/100!=2{b,_:=io.ReadAll(resp.Body);return fmt.Errorf("telegram %d: %s",resp.StatusCode,string(b))};return nil
}
func main(){
  rpcURL:=os.Getenv("ARC_RPC_URL"); if rpcURL==""{log.Fatal("ARC_RPC_URL is required")}; token:=os.Getenv("TELEGRAM_BOT_TOKEN");chat:=os.Getenv("TELEGRAM_CHAT_ID"); lookback:=envInt("LOOKBACK_BLOCKS",80); maxAlerts:=envInt("MAX_ALERTS",12)
  ctx,cancel:=context.WithTimeout(context.Background(),4*time.Minute);defer cancel();r:=&RPC{rpcURL,&http.Client{Timeout:20*time.Second}}
  if os.Getenv("TEST_ALERT")=="1"{if err:=telegram(ctx,token,chat,"✅ Arc Mainnet Monitor 测试成功\nGitHub Actions → Telegram 通道正常");err!=nil{log.Fatal(err)};return}
  var latestHex string;if err:=r.call(ctx,"eth_blockNumber",[]any{},&latestHex);err!=nil{log.Fatal(err)};latest,err:=hexUint(latestHex);if err!=nil{log.Fatal(err)};start:=uint64(0);if latest>uint64(lookback){start=latest-uint64(lookback)}
  found:=0
  for n:=start;n<=latest && found<maxAlerts;n++{var b Block;if err:=r.call(ctx,"eth_getBlockByNumber",[]any{fmt.Sprintf("0x%x",n),true},&b);err!=nil{log.Printf("block %d: %v",n,err);continue};for _,tx:=range b.Transactions{if tx.To!=nil{continue};var rc Receipt;if err:=r.call(ctx,"eth_getTransactionReceipt",[]any{tx.Hash},&rc);err!=nil||rc.ContractAddress==nil||*rc.ContractAddress==""||rc.Status=="0x0"{continue};name,symbol:=tokenMeta(ctx,r,*rc.ContractAddress);kind:="Contract";if name!=""||symbol!=""{kind="ERC20-like token"};msg:=fmt.Sprintf("🚨 ARC NEW %s\nBlock: %d\nName: %s\nSymbol: %s\nContract: %s\nDeployer: %s\nTx: %s\n\n⚠️ 仅链上新部署信号，不代表安全或建议买入。",kind,n,empty(name,"-"),empty(symbol,"-"),*rc.ContractAddress,tx.From,tx.Hash);if err:=telegram(ctx,token,chat,msg);err!=nil{log.Printf("telegram: %v",err)};found++;if found>=maxAlerts{break}}}
  log.Printf("done latest=%d start=%d alerts=%d",latest,start,found)
}
func empty(s,d string)string{if strings.TrimSpace(s)==""{return d};return s}
