## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. When in doubt, invoke the skill.

Key routing rules:

- Product ideas or brainstorming: invoke `/office-hours`.
- Strategy or scope: invoke `/plan-ceo-review`.
- Architecture: invoke `/plan-eng-review`.
- Design system or plan review: invoke `/design-consultation` or `/plan-design-review`.
- Full review pipeline: invoke `/autoplan`.
- Bugs or errors: invoke `/investigate`.
- QA or testing site behavior: invoke `/qa` or `/qa-only`.
- Review or diff review: invoke `/review` or `/design-review`.
- Ship, deploy, or create a PR: invoke `/ship` or `/land-and-deploy`.
- Save or restore working context: invoke `/context-save` or `/context-restore`.
- Turn work into a backlog-ready spec or issue: invoke `/spec`.
