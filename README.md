# pinger

周期性的测试目标服务器延时，产生 prometheus metrics 数据。

## 命令 

```shell
pinger [-c config.yaml] 
```

## 配置文件

```yaml
listen: 127.0.0.1:8888
jobs:
  google:               # job_name 工作名
    type: tls           # ping 类型。{tcp|tls}。默认 tcp
    addr: www.google.com:443  # 目标地址
    interval: 1m        # ping 间隔。默认 1m。
  cloudflare:
    type: tcp
    addr: 1.1.1.1:443
```

## Metrics

```text
pinger_latency_seconds{job=<job_name>}
pinger_error_total{job=<job_name>}
```