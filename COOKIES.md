# Facebook Embeds Cookie Guide

This guide documents the cookies required by Facebed to scrape gated content (like Reels) and provides instructions on how to extract them from a browser session and parse them into the required `cookies.json` format.

---

## Required Cookies

For Facebed to bypass Facebook's login screens and age gates, the following cookies must be extracted from an authenticated Facebook session:

1. **`c_user`**: The Facebook User ID of the logged-in account.
2. **`xs`**: The session token/secret.
3. **`fr`**: Encrypted advertising and browser identifier.
4. **`datr`**: Long-term browser security identifier.
5. **`sb`**: Long-term browser authentication identifier.

> [!IMPORTANT]
> If any of these cookies expire or are missing, Facebed will not be able to scrape private or age-gated reels and posts, and it will emit a warning notification.

---

## Extraction Instructions for AI Agents

To obtain the current cookies, copy the request headers from your browser and let the agent convert them to `cookies.json`.

### Step 1: Copy Request from Browser
1. Open your browser, go to [facebook.com](https://www.facebook.com), and log in.
2. Open Developer Tools (press `F12` or right-click -> **Inspect**) and navigate to the **Network** tab.
3. Reload the page or navigate to a Reel.
4. Look for a network request to `www.facebook.com` (for example, a request named `graphql/` or the initial page request).
5. Right-click the request and choose:
   - **Copy** -> **Copy as Node.js fetch**

### Step 2: Feed the Copied Request to the Agent
Provide the copied code to the AI agent. The copied code will contain a `headers` object with a `cookie` attribute like this:

```javascript
fetch("https://www.facebook.com/api/graphql/", {
  "headers": {
    "accept": "*/*",
    "cookie": "sb=6C_jZ...; datr=6C_jS...; c_user=100030...; fr=116xKK...; xs=18%3Amu...",
    "user-agent": "Mozilla/5.0..."
  },
  "method": "POST"
});
```

### Step 3: Run the Conversion Code

The AI agent (or you) can extract the `cookie` header string and parse it using one of the scripts below.

#### NodeJS Parser Script
Create a file `parse_cookies.js` or run this directly in the console:

```javascript
const fs = require('fs');

// 1. Paste the raw cookie header string here
const rawCookieString = "sb=6C_jZ...; datr=6C_jS...; c_user=100030...; fr=116xKK...; xs=18%3Amu...";

// 2. Parse the string into structured cookies.json
const requiredNames = ['sb', 'datr', 'c_user', 'fr', 'xs'];
const parsedCookies = rawCookieString
  .split(';')
  .map(item => {
    const parts = item.trim().split('=');
    if (parts.length < 2) return null;
    const name = parts[0];
    const value = parts.slice(1).join('=');
    
    if (requiredNames.includes(name)) {
      return {
        name: name,
        value: value,
        expirationDate: 2147483647 // Set to far future (Year 2038)
      };
    }
    return null;
  })
  .filter(Boolean);

// 3. Write output to cookies.json
fs.writeFileSync('cookies.json', JSON.stringify(parsedCookies, null, 2));
console.log('Successfully wrote cookies to cookies.json!');
```

#### Python Parser Script
Alternatively, use this Python snippet:

```python
import json

# Paste the raw cookie header string here
raw_cookie_string = "sb=6C_jZ...; datr=6C_jS...; c_user=100030...; fr=116xKK...; xs=18%3Amu..."

required_names = {'sb', 'datr', 'c_user', 'fr', 'xs'}
parsed_cookies = []

for item in raw_cookie_string.split(';'):
    parts = item.strip().split('=', 1)
    if len(parts) < 2:
        continue
    name, value = parts
    if name in required_names:
        parsed_cookies.append({
            "name": name,
            "value": value,
            "expirationDate": 2147483647 # Year 2038
        })

with open('cookies.json', 'w') as f:
    json.dump(parsed_cookies, f, indent=2)

print("Successfully wrote cookies to cookies.json!")
```
