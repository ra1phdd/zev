# Setup Flow Example

## OpenAI Setup with Custom Base URL

When running `zev setup` and selecting OpenAI, the user will see:

```
? Which LLM provider would you like to use?
  > openai
    gemini
    ollama

Enter your OpenAI API key: sk-proj-***
Model (default: gpt-4-turbo): gpt-3.5-turbo
Base URL (default: https://api.openai.com/v1): 

✓ Configuration saved successfully!
```

### Use Cases:

#### 1. Default OpenAI
```
API Key: sk-proj-***
Model: [press Enter] → gpt-4-turbo
Base URL: [press Enter] → https://api.openai.com/v1
```

#### 2. Custom Model
```
API Key: sk-proj-***
Model: gpt-3.5-turbo
Base URL: [press Enter] → https://api.openai.com/v1
```

#### 3. OpenAI-Compatible Provider
```
API Key: sk-***
Model: llama-3-70b
Base URL: https://api.together.xyz/v1
```

#### 4. Local OpenAI Proxy
```
API Key: local
Model: gpt-4
Base URL: http://localhost:8000/v1
```

## Gemini Setup

```
? Which LLM provider would you like to use?
    openai
  > gemini
    ollama

Enter your Gemini API key: AI***
Model (default: gemini-3-flash-preview): 

✓ Configuration saved successfully!
```

## Ollama Setup

```
? Which LLM provider would you like to use?
    openai
    gemini
  > ollama

Base URL (default: http://localhost:11434): 
Model (e.g., llama2, mistral): llama2

✓ Configuration saved successfully!
```

### Ollama Custom URL
```
Base URL (default: http://localhost:11434): http://192.168.1.100:11434
Model (e.g., llama2, mistral): mistral

✓ Configuration saved successfully!
```

## Configuration File Result

After setup, `~/.zevrc` contains:

```yaml
llm_provider: openai
openai_api_key: ""
openai_model: gpt-4-turbo
openai_base_url: https://api.openai.com/v1
gemini_api_key: ""
gemini_model: gemini-3-flash-preview
ollama_base_url: ""
ollama_model: ""
```

And `~/.zevrc.security.yaml` contains secrets:

```yaml
openai_api_key: sk-proj-***
```

## Benefits

1. **Flexibility**: Users can easily switch between providers and custom endpoints
2. **Security**: API keys stored separately in security overlay
3. **Convenience**: Smart defaults for common configurations
4. **Clarity**: Clear prompts showing default values
5. **Compatibility**: Supports any OpenAI-compatible API (Together, Anyscale, local proxies)
