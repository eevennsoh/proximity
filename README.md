
# Mini-proxy

This is a small configurable proxy with the aim of being able to compile it to proxy anything. This was a small fun project with the aim of proxying AI-Gateway to use OSS AI tools with the models it provides.

Ai-Gateway rest documentation: https://developer.atlassian.com/platform/ai-gateway/rest/,

It requires [postman-slauthtoken](https://bitbucket.org/atlassian-developers/postman-slauthtoken/src/master/README.md) to get slauth tokens to properly authenticate to AI-Gateway.

You can run it locally using `make` or run the image docker the docker compose file provided.

### Routes available

The OpenAI chat endpoint currently available. You can plug this into any project that expects an OpenAI api and it will work.

```
http://localhost:3001/openai/v1/chat/completions
```

The models available can be found here: https://developer.atlassian.com/platform/ai-gateway/models/openai/.

The proxy currently only exposes an endpoint to proxy the OpenAI models because that worked out of the box. It's a bit more of a struggle with the other providers. Contributions are welcome to help expand the number of models available!
