package main

import (
 "bytes"; "context"; "encoding/hex"; "encoding/json"; "fmt"; "io"; "log"; "net/http"; "os"; "strconv"; "strings"; "time"
)

type RPC struct{ URL string; Client *http.Client }
type rpcResp struct{ Result json.RawMessage `json:"result"`; Error *struct{Code int `json:"code"`; Message string `json:"message"`} `json:"error"` }
type Block struct{ Number,Timestamp string; Transactions []Tx `json:"transactions"` }
type Tx struct{ Hash string `json:"hash"`; To *string `json:"to"`; From,Value,Input string }
type Receipt struct{ ContractAddress *string `json:"contractAddress"`; Status,GasUsed string }

func (r *RPC) call(ctx context.Context, method string, params any, out any) error {
 body,_:=json.Marshal(map[string]any{"jsonrpc":"2.0","id":1,"method":method,"params":params})
 var last error
 for attempt:=0;attempt<5;attempt++ { if attempt>0 { select{case <-ctx.Done():return ctx.Err();case <-time.After(time.Duration(1<<uint(attempt-1))*time.Second):} }
  req,err:=http.NewRequestWithContext(ctx,"POST",r.URL,bytes.NewReader(body));if err!=nil{return err};req.Header.Set("Content-Type","application/json")
  resp,err:=r.Client.Do(req);if err!=nil{last=err;continue};b,_:=io.ReadAll(resp.Body);resp.Body.Close()
  if resp.StatusCode/100!=2 { last=fmt.Errorf("rpc http %d: %s",resp.StatusCode,string(b)); if resp.StatusCode==429||resp.StatusCode>=500{continue};return last }
  var rr rpcResp;if err=json.Unmarshal(b,&rr);err!=nil{last=err;continue};if rr.Error!=nil{last=fmt.Errorf("rpc %d: %s",rr.Error.Code,rr.Error.Message);continue};return json.Unmarshal(rr.Result,out)
 };return last
}
func hexUint(s string)(uint64,error){return strconv.ParseUint(strings.TrimPrefix(s,"0x"),16,64)}
func envInt(k string,d int)int{v,_:=strconv.Atoi(os.Getenv(k));if v<=0{return d};return v}
func newBig(b []byte)uint64{var n uint64;start:=0;if len(b)>8{start=len(b)-8};for _,x:=range b[start:]{n=n<<8|uint64(x)};return n}
func decodeABIString(raw string)string{b,e:=hex.DecodeString(strings.TrimPrefix(raw,"0x"));if e!=nil||len(b)==0{return ""};if len(b)>=64{n:=int(newBig(b[32:64]));if n>0&&64+n<=len(b){return strings.TrimSpace(string(b[64:64+n]))}};if len(b)>=32{return strings.TrimSpace(strings.TrimRight(string(b[:32]),"\x00"))};return ""}
func ethCall(ctx context.Context,r *RPC,a,s string)string{var out string;if r.call(ctx,"eth_call",[]any{map[string]string{"to":a,"data":s},"latest"},&out)!=nil{return ""};return out}
func tokenMeta(ctx context.Context,r *RPC,a string)(string,string,string,string){return decodeABIString(ethCall(ctx,r,a,"0x06fdde03")),decodeABIString(ethCall(ctx,r,a,"0x95d89b41")),hexNum(ethCall(ctx,r,a,"0x313ce567")),hexNum(ethCall(ctx,r,a,"0x18160ddd"))}
func hexNum(s string)string{if s==""||s=="0x"{return "-"};n,e:=hexUint(s);if e!=nil{return "-"};return strconv.FormatUint(n,10)}
func telegram(ctx context.Context,t,c,text string)error{if t==""||c==""{return fmt.Errorf("telegram secrets missing")};u:="https://api.telegram.org/bot"+t+"/sendMessage";p,_:=json.Marshal(map[string]any{"chat_id":c,"text":text,"disable_web_page_preview":true});req,_:=http.NewRequestWithContext(ctx,"POST",u,bytes.NewReader(p));req.Header.Set("Content-Type","application/json");resp,e:=http.DefaultClient.Do(req);if e!=nil{return e};defer resp.Body.Close();if resp.StatusCode/100!=2{b,_:=io.ReadAll(resp.Body);return fmt.Errorf("telegram %d: %s",resp.StatusCode,string(b))};return nil}
func empty(s,d string)string{if strings.TrimSpace(s)==""{return d};return s}
func shortSupply(raw,dec string)string{if raw=="-"||dec=="-"{return raw};d,e:=strconv.Atoi(dec);if e!=nil||d>18{return raw};n,e:=strconv.ParseUint(raw,10,64);if e!=nil{return raw};x:=float64(n);for i:=0;i<d;i++{x/=10};return fmt.Sprintf("%.4g",x)}

func main(){
 rpcURL:=os.Getenv("ARC_RPC_URL");if rpcURL==""{log.Fatal("ARC_RPC_URL is required")};token,chat:=os.Getenv("TELEGRAM_BOT_TOKEN"),os.Getenv("TELEGRAM_CHAT_ID");lookback,maxAlerts:=envInt("LOOKBACK_BLOCKS",80),envInt("MAX_ALERTS",12)
 ctx,cancel:=context.WithTimeout(context.Background(),4*time.Minute);defer cancel();r:=&RPC{rpcURL,&http.Client{Timeout:20*time.Second}}
 if os.Getenv("TEST_ALERT")=="1"{if e:=telegram(ctx,token,chat,"✅ Arc Mainnet Monitor 测试成功");e!=nil{log.Fatal(e)};return}
 var chain string;if e:=r.call(ctx,"eth_chainId",[]any{},&chain);e!=nil{log.Fatal(e)};if strings.ToLower(chain)!="0x13b2"{log.Fatalf("wrong chain id %s, expected 0x13b2 (5042)",chain)}
 var lh string;if e:=r.call(ctx,"eth_blockNumber",[]any{},&lh);e!=nil{log.Fatal(e)};latest,e:=hexUint(lh);if e!=nil{log.Fatal(e)};start:=uint64(0);if latest>uint64(lookback){start=latest-uint64(lookback)};found,failed:=0,0
 for n:=start;n<=latest&&found<maxAlerts;n++{var b Block;if e:=r.call(ctx,"eth_getBlockByNumber",[]any{fmt.Sprintf("0x%x",n),true},&b);e!=nil{failed++;log.Printf("block %d FAILED after retries: %v",n,e);continue};ts,_:=hexUint(b.Timestamp)
  for _,tx:=range b.Transactions{if tx.To!=nil{continue};var rc Receipt;if e:=r.call(ctx,"eth_getTransactionReceipt",[]any{tx.Hash},&rc);e!=nil{failed++;continue};if rc.ContractAddress==nil||*rc.ContractAddress==""||rc.Status=="0x0"{continue}
   addr:=*rc.ContractAddress;name,symbol,dec,supplyRaw:=tokenMeta(ctx,r,addr);kind:="Contract";if name!=""||symbol!=""||dec!="-"{kind="ERC20-like Token"};var code string;_=r.call(ctx,"eth_getCode",[]any{addr,"latest"},&code);codeBytes:=len(strings.TrimPrefix(code,"0x"))/2;gas:=hexNum(rc.GasUsed);age:="-";if ts>0{age=time.Since(time.Unix(int64(ts),0)).Round(time.Second).String()}
   msg:=fmt.Sprintf("🚨 ARC NEW %s\n\n🏷 Name: %s\n🔤 Symbol: %s\n🔢 Decimals: %s\n🪙 Total Supply: %s\n\n📦 Block: %d\n⏱ Age: %s\n📄 Contract: %s\n👤 Deployer: %s\n⛽ Deploy Gas: %s\n💾 Code Size: %d bytes\n\n🔎 Contract: https://arc-scan.org/address/%s\n🔎 Tx: https://arc-scan.org/tx/%s\n\n⚠️ 新部署仅代表链上活动；未验证源码、流动性、持仓集中度、交易权限或安全性。",kind,empty(name,"-"),empty(symbol,"-"),dec,shortSupply(supplyRaw,dec),n,age,addr,tx.From,gas,codeBytes,addr,tx.Hash)
   if e:=telegram(ctx,token,chat,msg);e!=nil{log.Printf("telegram: %v",e)};found++;if found>=maxAlerts{break}
  }
 }
 log.Printf("done chain=5042 latest=%d start=%d alerts=%d failed_rpc_items=%d",latest,start,found,failed);if failed>0{log.Fatalf("scan incomplete: %d RPC items failed after retries",failed)}
}
