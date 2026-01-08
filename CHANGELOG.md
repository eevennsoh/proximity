
### Breaking Changes
- **Profiles**: Added support for using multiple use-cases. Profiles define an Atlassian Cloud ID, use case ID, and AD group. Each let's you route requests to AI-Gateway using it's own configuration.

### Bug Fixes
- Fixed the slauth token environment the match the AI-Gateway environment.
- Fixed an issue where logging pipe doesn't close and sometimes prevent starting the proxy after it was stopped.
- Added the Edit menu to enable Cmd+C copy functionality in the logs panel.
