# Agent Deploy

Allow for natural language deployment of applications.

User -> Agent -> MCP server -> Cloud provider

The goal is to allow users to end-to-end create applications and make them available publically.

## The loop

Prerequisite: User provides us with cloud provider keys

1. User vibe codes something
2. User wants to make it available
3. User tells the agent to deploy
3a. Agent askes clarifying questions (how many users, what kind of latency, where are they located, etc.)
3b. Agent responds with a plan and a price estimation
3c. User approves the plan
4. Agent makes calls to cloud provider to stand up infra
5. Agent responds to user with details about their deployment


## Safe guards

Ensure spend does not cross some boundry set by the user
