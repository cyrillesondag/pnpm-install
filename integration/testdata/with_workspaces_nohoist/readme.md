Sample monorepo application. It has 2 packages - express app (`packages/sample-app`) and separate config package (`packages/sample-config`). Express application uses config package. 

**Prerequisites**
- Node `8.11.x`
- Pnpm `9.x`

**Build project**
```
pnpm install
```

**Run locally**
```
pnpm start:app
```

**Smoke test locally**

Locally URL `http://localhost:3000/check` should return valid JSON: 
```
{
    "config": {
        "prop1": "value1",
        "prop2": "value2"
    }
}
```
