<div align="center">

  <h1>🃏 Elvis</h1>
  <h3>webOS AI Home Agent</h3>

  <p>
    <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/Platform-webOS%20%7C%20ARM64-blue" alt="Platform">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
  </p>

</div>

---

[PicoClaw](https://github.com/sipeed/picoclaw) 포크 기반 **webOS TV/IoT 전용 AI 홈 에이전트**.

Rule Engine이 LLM처럼 행세하면서 90%의 홈 커맨드를 API 호출 없이 처리하고, 못 잡는 건 Cloud LLM이 같은 스킬로 처리한다.

## 빌드

```bash
git clone https://github.com/rordd/elvis.git
cd elvis
make deps && make build
```

## 문서

- [아키텍처](project-elvis-architecture.md)
- [로드맵](project-elvis-roadmap.md)
- [PRD](project-elvis-prd.md)

## 라이선스

MIT
