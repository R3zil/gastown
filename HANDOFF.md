# 🤝 HANDOFF: Deacon Heartbeat MOTD Feature

## Current State
Just finished a productive session enhancing all tmux status bars with:
- Mail subject previews (instead of just counts)
- Current work display (in_progress issues from beads)
- Compact formatting (removed redundant role icons)
- Cat emoji (😺) for polecat counts

All pushed to main.

## Next Task: Deacon Heartbeat MOTD

The deacon gets a heartbeat/nudge every ~minute. It's a thankless patrol role.
Make it more fun by adding rotating motivational/educational messages.

### Requirements
1. Add MOTD (Message of the Day) tips to deacon heartbeat
2. Rotate through messages - don't repeat consecutively
3. Mix of:
   - Gratitude: "Thanks for everything you do!"
   - Encouragement: "This is Gas Town's most critical role."
   - Inspiration: "You are the heart of Gas Town! Be watchful!"
   - Educational tips about Gas Town architecture and theory of operation

### Where to Look
- Deacon heartbeat logic: likely in `internal/daemon/` or `internal/cmd/deacon.go`
- Look for where nudges/heartbeats are sent to deacon
- The message appears in the deacon's tmux pane

### Example Messages to Include
- "Thanks for keeping the town running!"
- "You are Gas Town's most critical role."
- "You are the heart of Gas Town! Be watchful!"
- "Tip: Polecats are ephemeral workers - spawn freely, kill liberally."
- "Tip: Witnesses monitor polecats; you monitor witnesses."
- "Tip: The Refinery handles merge conflicts so polecats don't have to."
- "Tip: Beads track work across all agents via git-synced issues."
- "Tip: Mail routes through town-level beads at ~/gt/.beads/"
- "Tip: Wisps are ephemeral molecules for patrol cycles - never synced."
- "Tip: Each rig has its own Witness, Refinery, and beads."

### Implementation Ideas
- Create a slice of messages in a new file (e.g., `internal/deacon/motd.go`)
- Use time-based or counter-based rotation
- Maybe store last index in a file to avoid repeats across restarts
- Or just random selection with simple dedup (don't repeat last message)

### Also Note
There's a sqlite migration bug in the mail system - "duplicate column name: replies_to".
May need to fix that or delete the db to reset.

Good luck! 🦉
