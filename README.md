<div align="center">

  <h1>🃏 Elvis — webOS AI Home Agent</h1>

  <h3>Rule Engine First · Cloud LLM Fallback · Zero API Cost for 90% Commands</h3>

  <p>
    <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/Platform-webOS%20%7C%20ARM64-blue" alt="Platform">
    <img src="https://img.shields.io/badge/Base-PicoClaw%20Fork-orange" alt="Fork">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
  </p>

</div>

---

## Elvis가 뭔데?

Elvis는 **webOS TV/IoT 전용 AI 홈 에이전트**다. [PicoClaw](https://github.com/sipeed/picoclaw) 포크 기반으로, "가짜 LLM"이 진짜 LLM처럼 행세하면서 대부분의 홈 커맨드를 API 호출 없이 처리한다.

설치하면 만나게 되는 건 **리사(Lisa)** — 밝고 에너지 넘치는 홈 에이전트. PicoClaw의 범용 어시스턴트가 아니라, TV 켜고 에어컨 온도 맞추고 세탁기 돌리는 **집 전용 AI**다.

## 핵심 아이디어

```
"볼륨 올려" → Rule Engine (regex 매칭) → volume.sh → 완료. API 비용 0원.
"거실 TV 소리 좀 줄여줄래?" → Rule Engine 실패 → Gemini Flash → 같은 volume.sh → 완료. API 1회.
```

**같은 스킬, 다른 두뇌.** Rule Engine이든 LLM이든 실행하는 건 동일한 `.skill` 패키지다.

| 구성요소 | Rule Engine | Cloud LLM |
|----------|------------|-----------|
| SKILL.md 설명 | 무시 (regex만 봄) | 시스템 프롬프트에 주입 |
| @actions 테이블 | Skill Resolver가 파싱 → exec | LLM이 읽고 tool call 결정 |
| *.sh 스크립트 | exec 직접 실행 | exec 직접 실행 |
| rules.json | regex 패턴 매칭 | 무시 |

> **"AI를 안 쓰는 AI"** — 반복 패턴을 학습해서 룰로 승격시키면, 결국 클라우드 없이도 집이 돌아간다.

## 아키텍처

```
┌──────────────────────────────────────────┐
│              Elvis (PicoClaw Fork)        │
│                                          │
│  ┌──────────┐  ┌──────────┐              │
│  │ Telegram │  │WebSocket │              │
│  │ Channel  │  │ Chat UI  │              │
│  └────┬─────┘  └────┬─────┘              │
│       └──────┬───────┘                   │
│              ▼                           │
│  ┌─────────────────────────┐             │
│  │      Agent Loop         │             │
│  └──────────┬──────────────┘             │
│             ▼                            │
│  ┌─────────────────────────┐             │
│  │    Fallback Chain       │             │
│  └──┬──────────────┬──────┘             │
│     ▼              ▼                     │
│  ┌──────────┐  ┌────────────┐            │
│  │  Rule    │  │ Gemini 2.5 │            │
│  │  Engine  │  │ Flash      │            │
│  │ (1순위)  │  │ (fallback) │            │
│  └──┬───────┘  └────────────┘            │
│     ▼                                    │
│  ┌─────────────────────────┐             │
│  │   Skill Resolver        │             │
│  │  .skill → exec tool     │             │
│  └─────────────────────────┘             │
└──────────────────────────────────────────┘
```

## 현재 스킬

| 스킬 | 명령 예시 | 처리 |
|------|----------|------|
| TV 채널 | "채널 5번 틀어" | Rule Engine |
| TV 볼륨 | "볼륨 올려/내려" | Rule Engine |
| TV 전원 | "TV 꺼/켜" | Rule Engine |
| 에어컨 | "에어컨 켜", "온도 24도" | Rule Engine |
| 세탁기 | "세탁기 돌려" | Rule Engine |
| 일반 대화 | "오늘 날씨 어때?" | Gemini Flash |

## 빌드

```bash
git clone https://github.com/rordd/elvis.git
cd elvis
make deps && make build
```

바이너리: `bin/picoclaw` (~25MB, Go static binary)

## 설정

`~/.picoclaw/config.json`:

```json
{
  "agents": {
    "defaults": {
      "model_name": "ruleengine/local",
      "max_tokens": 65536
    }
  },
  "model_list": [
    {
      "model_name": "ruleengine/local",
      "model": "ruleengine/local",
      "api_base": "<rules.json 경로>",
      "workspace": "<skills 디렉토리>"
    },
    {
      "model_name": "gemini/gemini-2.5-flash",
      "model": "gemini/gemini-2.5-flash"
    }
  ]
}
```

> Gemini API 키는 `GEMINI_API_KEY` 환경변수로 설정.

## 로드맵

| Phase | 내용 |
|-------|------|
| **1** | Synapse Engine (자가학습), 원클릭 배치, 음성대화 |
| **2** | 홈 시뮬레이터 (가상 집 + MQTT) |
| **3** | Jarvis UI (TV 인터페이스 + A2UI) |
| **4** | webOS TV 이식 (TV 단독 동작) |
| **5** | 동적 배치 (가전 디스커버리) |
| **6** | Extended Brain (멀티에이전트 분산) |
| **7** | 자율형 에이전트 (Lv.0→Lv.4) |
| **8** | 보안 & 가드레일 |

자세한 내용: [`project-elvis-roadmap.md`](project-elvis-roadmap.md)

## 기반

[PicoClaw](https://github.com/sipeed/picoclaw) — Ultra-Efficient AI Assistant in Go by [Sipeed](https://sipeed.com)

Elvis는 PicoClaw의 포크이며, upstream 변경사항을 주기적으로 머지합니다.

## 라이선스

MIT
