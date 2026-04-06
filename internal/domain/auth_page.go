package domain

import (
	"fmt"
	"os"
	"path/filepath"
)

const authPageHTML = `<!DOCTYPE html>
<html lang="cs">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Stránka není dostupná</title>
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

    .access-toggle {
      background: none;
      border: none;
      color: #52525b;
      font-size: 0.8rem;
      cursor: pointer;
      padding: 0.25rem 0.5rem;
      border-radius: 4px;
      transition: color 0.15s;
      letter-spacing: 0.05em;
      text-transform: uppercase;
    }
    .access-toggle:hover { color: #a1a1aa; }

    .login-form {
      display: none;
      margin-top: 1.25rem;
    }
    .login-form.visible { display: block; }

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
      font-size: 0.9rem;
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
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
    </svg>

    <h1>Stránka není dostupná</h1>
    <p>Omlouváme se, ale tato stránka je dočasně nedostupná.<br>Zkuste to prosím později.</p>

    <hr class="divider" />

    <button class="access-toggle" onclick="toggleLogin()">Přístup</button>

    <div class="login-form" id="loginForm">
      <form method="POST" action="/_deployik/verify" id="authForm">
        <div class="input-group">
          <input type="password" name="password" id="passwordInput" placeholder="Heslo" autocomplete="current-password" />
          <button type="submit" id="submitBtn">Vstoupit</button>
        </div>
        <div class="error-msg" id="errorMsg">Nesprávné heslo. Zkuste to znovu.</div>
      </form>
    </div>
  </div>

  <script>
    function toggleLogin() {
      var form = document.getElementById('loginForm');
      form.classList.toggle('visible');
      if (form.classList.contains('visible')) {
        document.getElementById('passwordInput').focus();
      }
    }
    // Show error if redirected back with ?error=1
    if (window.location.search.indexOf('error=1') !== -1) {
      document.getElementById('loginForm').classList.add('visible');
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
