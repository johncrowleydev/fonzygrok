---
id: DEF-003
title: "Tunnel proxy returns unexpected EOF over internet"
type: defect
status: OPEN
severity: CRITICAL
owner: architect
agents: [developer-a, developer-b]
tags: [defect, proxy, networking, v1.1]
related: [DEF-002, SPR-015]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# DEF-003: Tunnel Proxy Returns `unexpected EOF` Over Internet

## Summary

Tunnel proxying works on localhost but fails when client and server are on
different machines over the internet. The server logs repeated
`edge: read response from tunnel: unexpected EOF` errors.

## Reproduction

1. Server running on EC2 (3.139.160.15, Docker)
2. Client running on Windows (home network)
3. Client connects successfully, tunnel registers
4. Browser hits `http://my-app.tunnel.fonzygrok.com`
5. Server receives request, opens proxy channel to client
6. **Server gets `unexpected EOF` reading response from SSH channel**
7. Browser gets 502 Bad Gateway

## Server Logs

```
{"level":"INFO","msg":"ssh: client connected","token_id":"tok_95e451e01203"}
{"level":"INFO","msg":"tunnel: registered","tunnel_id":"grbls1","name":"my-app"}
{"level":"ERROR","msg":"edge: read response from tunnel","tunnel_id":"grbls1","error":"unexpected EOF"}
{"level":"ERROR","msg":"edge: read response from tunnel","tunnel_id":"grbls1","error":"unexpected EOF"}
... (repeats for every request)
```

## Analysis

The proxy works on localhost because io.Copy completes instantly with no
network latency. Over the internet:

1. Server writes HTTP request to SSH channel, then calls CloseWrite()
   (edge.go:267-269)
2. Client reads request from channel, forwards to local service
3. Local service responds
4. Client writes response back to SSH channel
5. **Server's http.ReadResponse gets EOF before the response arrives**

Possible causes:
- CloseWrite() on the server side may be signaling end-of-stream to the
  client before the client finishes processing
- The bidirectional copy in client proxy (proxy.go:128-149) may have a race
  condition — the wg.Wait() blocks but the SSH channel may close prematurely
- Buffering differences over a real TCP connection vs localhost
- The TeeReader wrapping for inspector capture may be interfering

## Required Fix

Investigate and fix the proxy pipeline to work reliably over real network
connections. The fix must be verified by connecting a client on a DIFFERENT
machine from the server, not just localhost.
