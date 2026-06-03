package domain

import (
	"fmt"
	"os"
	"path/filepath"
)

const authPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Password protected</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      background: #0a0a0a;
      color: #a1a1aa;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 2rem;
    }

    .container {
      max-width: 420px;
      width: 100%;
      text-align: center;
    }

    .icon {
      width: 48px;
      height: 48px;
      margin: 0 auto 1.5rem;
      color: #3f3f46;
    }

    h1 {
      font-size: 1.25rem;
      font-weight: 600;
      color: #e4e4e7;
      margin-bottom: 0.75rem;
    }

    p {
      font-size: 0.9rem;
      line-height: 1.6;
      color: #71717a;
    }

    .divider {
      border: none;
      border-top: 1px solid #1f1f23;
      margin: 2.5rem 0 1.5rem;
    }

    .login-form {
      margin-top: 1.5rem;
    }

    .input-group {
      display: flex;
      gap: 0.5rem;
      align-items: center;
    }

    input[type="password"] {
      flex: 1;
      background: #18181b;
      border: 1px solid #27272a;
      border-radius: 6px;
      color: #e4e4e7;
      font-size: 16px; /* 16px (not <16) prevents iOS Safari zoom-on-focus */
      padding: 0.5rem 0.75rem;
      outline: none;
      transition: border-color 0.15s;
    }
    input[type="password"]:focus { border-color: #52525b; }
    input[type="password"]::placeholder { color: #3f3f46; }

    button[type="submit"] {
      background: #27272a;
      border: 1px solid #3f3f46;
      border-radius: 6px;
      color: #e4e4e7;
      font-size: 0.85rem;
      padding: 0.5rem 1rem;
      cursor: pointer;
      transition: background 0.15s;
      white-space: nowrap;
    }
    button[type="submit"]:hover { background: #3f3f46; }
    button[type="submit"]:disabled { opacity: 0.5; cursor: not-allowed; }

    .error-msg {
      display: none;
      color: #ef4444;
      font-size: 0.8rem;
      margin-top: 0.75rem;
    }
    .error-msg.visible { display: block; }
  </style>
</head>
<body>
  <div class="container">
    <svg class="icon" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
    </svg>

    <h1>This page is password protected</h1>
    <p>This content is private. Enter the password to continue.</p>

    <hr class="divider" />

    <form class="login-form" method="POST" action="/_deployik/verify" id="authForm">
      <div class="input-group">
        <input type="password" name="password" id="passwordInput" placeholder="Password" autocomplete="current-password" autofocus />
        <button type="submit" id="submitBtn">Continue</button>
      </div>
      <div class="error-msg" id="errorMsg">Incorrect password. Please try again.</div>
    </form>
  </div>

  <script>
    // Show error if redirected back with ?error=1
    if (window.location.search.indexOf('error=1') !== -1) {
      document.getElementById('errorMsg').classList.add('visible');
    }
  </script>
</body>
</html>
`

// WriteAuthPage writes the static auth HTML page to the given directory.
// The file is created at dir/auth.html.
func WriteAuthPage(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create auth-pages directory: %w", err)
	}

	path := filepath.Join(dir, "auth.html")
	if err := os.WriteFile(path, []byte(authPageHTML), 0644); err != nil {
		return fmt.Errorf("write auth page: %w", err)
	}

	return nil
}
