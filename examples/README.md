# AgentFlow examples

Complete, runnable chatbot deployments matching Studio's built-in templates: a config, its TypeScript loop, persona prompt, `.env.example`, and README each. Run any of them from the repository root with `agentflow run -config examples/<example>/agentflow.yaml`.

| Example | Title | What it shows |
| --- | --- | --- |
| [`qq-group-bot-fts/`](qq-group-bot-fts/) | QQ Group Chatbot (FTS5) | QQ group & C2C chatbot with SQLite FTS5 keyword recall (`plugin:recency`), `session.state` context window, proactive `[NO_REPLY]` participation, Doubao `web_search`, and `fetch`. |
| [`qq-group-bot-vec/`](qq-group-bot-vec/) | QQ Group Chatbot (Embedding) | QQ group & C2C chatbot with `bge-m3` SQLite vector semantic recall (`plugin:semantic`), `session.state` context window, proactive `[NO_REPLY]` participation, Doubao `web_search`, and `fetch`. |
| [`telegram-group-bot-fts/`](telegram-group-bot-fts/) | Telegram Group Chatbot (FTS5) | Telegram group & DM chatbot with SQLite FTS5 keyword recall (`plugin:recency`), `session.state` context window, proactive `[NO_REPLY]` participation, Google/X `web_search`, `fetch`, and `browser`. |
| [`telegram-group-bot-vec/`](telegram-group-bot-vec/) | Telegram Group Chatbot (Embedding) | Telegram group & DM chatbot with `bge-m3` SQLite vector semantic recall (`plugin:semantic`), `session.state` context window, proactive `[NO_REPLY]` participation, Google/X `web_search`, `fetch`, and `browser`. |
