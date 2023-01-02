# pinger

- 实现 UDP 协议 ping 。
- 支持简单的 UDP 包头验证，避免暴露的端口被滥用。
- 支持延时/丢包率监控。周期性的测试目标服务器，支持 prometheus metrics 。

## 命令 

```shell
# 使用 UDP 协议 ping 服务器。
# -a 服务器密码。可选。
# -n ping 次数。默认 4 次。
# -n ping 间隔。默认 1s 。
# -f flood ping 。
pinger uping [-a server_auth] [-n number_of_ping] [-i interval] [-f] 127.0.0.1:48330 

# 启用服务端，应答 UDP ping。
# -a 服务器密码。可选。如果收到密码不对(包头不对)的数据包则不会回应。
pinger userver [-a auth] -l :48330

# 监控服务器的 ping 情况，产生 prometheus metrics 数据。
pinger watcher -c pinger-watcher.yaml 
```

## Watcher

```yaml
listen: 127.0.0.1:8888  # metrics http server
jobs:
  cloudflare:           # job name
    type: tcp           # ping type, one of {udp|tcp|cmd}
    addr: 1.1.1.1:443   # target ip:port address
    interval: 1m        # ping interval, default is 1m
    auth: ""            # udp ping auth password, optional

    exec: ""            # cmd ping executable
    args:               # cmd ping args
      - ""
```

- tcp ping 即常见的 tcp syn ack ping。
- cmd ping 会执行外部命令 `exec args...`。外部命令需将延时结果输出至 stdout 。示例: `53ms`, `0.053s` 。

输出以下 prometheus metrics:

```text
pinger_latency_seconds{job=<job_name>}
pinger_error_total{job=<job_name>}
```