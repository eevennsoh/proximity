
### New Features
- **AI Eval Gateway**: Added a selectable AI Gateway target for eval and offline workloads, routing requests to ai-eval-gateway with the matching SLAuth audience. (#18, @gdebellis)

### Minor Improvements
- **Health Check Endpoint**: Added a health check endpoint on the AI Gateway proxy implementations.
- **Log to File**: Added an option to write logs to a file in the `atlas` cli command.
- **Support Button**: Added a support button with tooltip to the UI.
- **Config-Driven Expr Logging**: Added the ability to log from expr expressions defined in config.
