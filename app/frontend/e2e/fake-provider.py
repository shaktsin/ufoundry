import json, http.server, sys
calls = {"n": 0}
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path.endswith("/models"):
            body = json.dumps({"data":[{"id":"local-llama"},{"id":"local-qwen"}]}).encode()
            self.send_response(200); self.send_header("Content-Type","application/json"); self.end_headers(); self.wfile.write(body)
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0)); req = json.loads(self.rfile.read(n))
        calls["n"] += 1
        key = (self.headers.get("Authorization") or "").split()[-1]
        # The "primary" key is rate limited, so the router has to move to the
        # backup one. Titles are exempt so the thread still gets named.
        user_text_all = " ".join(str(m.get("content")) for m in req["messages"] if m["role"] == "user")
        model = req.get("model", "")
        # "drop down" asks both keys of the first model to rate limit, so the
        # router has to change model, not just key.
        limited = key == "sk-primary-key" or ("drop down" in user_text_all and model == "local-llama")
        if limited and "3-6 word" not in user_text_all:
            body = json.dumps({"error": {"message": "rate limit reached"}}).encode()
            self.send_response(429); self.send_header("Content-Type", "application/json")
            self.send_header("Retry-After", "120"); self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return
        self.send_response(200); self.send_header("Content-Type","text/event-stream"); self.end_headers()
        def ev(o): self.wfile.write(("data: "+json.dumps(o)+"\n\n").encode())
        last = req["messages"][-1]
        user_text = next((m.get("content") for m in reversed(req["messages"]) if m["role"]=="user"), "")
        if "title" in str(user_text).lower() and "3-6 word" in str(user_text):
            ev({"choices":[{"delta":{"content":"Listing Workspace Files"}}]})
            ev({"choices":[],"usage":{"prompt_tokens":40,"completion_tokens":5}})
        elif last["role"] == "user" and "run" in str(last.get("content")):
            ev({"choices":[{"delta":{"tool_calls":[{"index":0,"id":"r1","function":{"name":"shell__run","arguments":"{\"command\":\"echo approved-run\"}"}}]}}]})
            ev({"choices":[],"usage":{"prompt_tokens":200,"completion_tokens":9}})
        elif last["role"] == "tool" and "approved-run" in str(last.get("content")):
            ev({"choices":[{"delta":{"content":"The command printed approved-run."}}]})
            ev({"choices":[],"usage":{"prompt_tokens":220,"completion_tokens":7}})
        elif last["role"] == "tool" and "denied" in str(last.get("content")):
            ev({"choices":[{"delta":{"content":"OK, I did not run it."}}]})
            ev({"choices":[],"usage":{"prompt_tokens":220,"completion_tokens":7}})
        elif last["role"] == "user" and "list" in str(last.get("content")):
            ev({"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"file__write","arguments":"{\"path\":\"notes.txt\",\"content\":\"one\\ntwo\\nthree\\n\"}"}}]}}]})
            ev({"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":300,"completion_tokens":20}})
        elif last["role"] == "tool":
            prev = [m for m in req["messages"] if m["role"]=="assistant" and m.get("tool_calls")][-1]["tool_calls"][0]["function"]["name"]
            if prev == "file__write":
                ev({"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c2","function":{"name":"file__list","arguments":"{}"}}]}}]})
                ev({"choices":[],"usage":{"prompt_tokens":350,"completion_tokens":10}})
            else:
                ev({"choices":[{"delta":{"content":"Wrote notes.txt; the workspace now has: " + last["content"].split(chr(9))[1]}}]})
                ev({"choices":[],"usage":{"prompt_tokens":400,"completion_tokens":12,"prompt_tokens_details":{"cached_tokens":300}}})
        else:
            for w in ["Hello ", "from ", "the ", "fake ", "model."]:
                ev({"choices":[{"delta":{"content":w}}]})
            ev({"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5}})
        self.wfile.write(b"data: [DONE]\n\n")
http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
