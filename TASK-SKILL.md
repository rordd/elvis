# Task: Refactor Rule Engine to skill-based architecture + create TV control skill

## Part 1: Create TV Control Skill

### Create ~/.picoclaw/workspace/skills/tv-control/SKILL.md
```markdown
# TV Control Skill

TV를 제어하는 스킬입니다. 채널 변경, 볼륨 조절, 전원 제어 등을 지원합니다.

## Actions
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| tv.channel.set | ./set_channel.sh {{channel}} | channel:number | 채널 변경 |
| tv.volume.up | ./volume.sh up | | 볼륨 올리기 |
| tv.volume.down | ./volume.sh down | | 볼륨 내리기 |
| tv.volume.set | ./volume.sh set {{level}} | level:number | 볼륨 설정 |
| tv.power.on | ./power.sh on | | TV 켜기 |
| tv.power.off | ./power.sh off | | TV 끄기 |
| tv.power.toggle | ./power.sh toggle | | TV 전원 전환 |
| tv.input.set | ./input.sh {{source}} | source:string | 입력 소스 전환 |
<!-- @end -->
```

### Create mock shell scripts (each prints what it would do):
- `set_channel.sh` — echo "[TV] 채널을 $1번으로 변경합니다."
- `volume.sh` — echo "[TV] 볼륨: $1" (up/down/set with $2)
- `power.sh` — echo "[TV] 전원: $1"
- `input.sh` — echo "[TV] 입력 소스: $1"
Make all executable (chmod +x).

### Create ~/.picoclaw/workspace/skills/iot-control/SKILL.md
Same pattern for IoT devices:
<!-- @actions -->
| intent | command | params | description |
|--------|---------|--------|-------------|
| aircon.power.on | ./aircon.sh on | | 에어컨 켜기 |
| aircon.power.off | ./aircon.sh off | | 에어컨 끄기 |
| aircon.temp.set | ./aircon.sh temp {{temp}} | temp:number | 에어컨 온도 설정 |
| washer.start | ./washer.sh start | | 세탁기 시작 |
| washer.stop | ./washer.sh stop | | 세탁기 정지 |
| fridge.temp.set | ./fridge.sh temp {{temp}} | temp:number | 냉장고 온도 설정 |
<!-- @end -->

Create mock scripts for each.

## Part 2: Refactor Rule Engine

### Modify pkg/providers/ruleengine/rules.go
- Add `Skill` field to Rule struct (json: "skill")
- Remove ToolCalls from Rule struct (no longer needed)
- Add SkillResolver that:
  1. Reads SKILL.md from skill directory
  2. Parses `<!-- @actions -->` table
  3. Finds command by intent
  4. Substitutes params into command template
  5. Caches parsed actions (don't re-read file every time)

### Modify pkg/providers/ruleengine/provider.go
- On match: use SkillResolver to get command from SKILL.md @actions table
- Build shell tool_call with the resolved command
- Return LLMResponse with content (response text) + tool_calls (shell command)
- Re-enable tool_calls (was disabled for mock mode)
- The tool_call should use the "shell" tool name with "command" argument

### Update rules.json (workspace/skills/ruleengine/rules.json)
Use new format:
- Remove old tool_calls
- Add "skill" field pointing to skill directory name
- Add "intent" field matching @actions table
- Keep patterns and response

Example:
```json
[
  {
    "id": "tv_channel_set",
    "intent": "tv.channel.set",
    "patterns": ["채널\\s*(?P<channel>\\d+)\\s*(?:번|로)?\\s*(?:틀어|변경|바꿔)?", "(?P<channel>\\d+)번\\s*(?:틀어|보여|봐)"],
    "skill": "tv-control",
    "response": "채널을 {{channel}}번으로 변경합니다.",
    "source": "manual"
  },
  {
    "id": "tv_volume_up",
    "intent": "tv.volume.up", 
    "patterns": ["볼륨\\s*(?:올려|높여|키워|up)", "소리\\s*(?:올려|높여|키워|크게)"],
    "skill": "tv-control",
    "response": "볼륨을 올립니다.",
    "source": "manual"
  },
  ... (all 7 original rules converted to new format)
]
```

## Part 3: Skill path configuration
- The skill resolver needs to know where skills live
- Read from PicoClaw workspace path: ~/.picoclaw/workspace/skills/
- Add SkillsDir field to RuleEngine config in pkg/config/config.go

## Part 4: Build & test
- go generate ./cmd/picoclaw/ && go build -o picoclaw-elvis ./cmd/picoclaw
- Run tests
- Git commit: "refactor: skill-based rule engine with @actions table"

## IMPORTANT
- The shell tool in PicoClaw is called "shell" and takes "command" as argument
- Tool call format must match what LLM would return
- SkillResolver should cache parsed @actions (sync.RWMutex)
- Handle missing skill gracefully (fallback to Cloud LLM)
