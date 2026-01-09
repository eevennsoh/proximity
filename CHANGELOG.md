
### New Features
- **Update Notifications**: A system notification is now sent on startup if a new version of Proximity is available.
- **Dynamic Model Lists**: The /models endpoints now fetch live model data from the ML Platform service, returning only allowlisted models for your configured use-case.

### Breaking Changes
- **Profiles**: Added support for using multiple use-cases. Profiles define an Atlassian Cloud ID, use case ID, and AD group. Each let's you route requests to AI-Gateway using it's own configuration.

### Bug Fixes
- Fixed the slauth token environment the match the AI-Gateway environment.
- Fixed an issue where logging pipe doesn't close and sometimes prevent starting the proxy after it was stopped.
- Added the Edit menu to enable Cmd+C copy functionality in the logs panel.
