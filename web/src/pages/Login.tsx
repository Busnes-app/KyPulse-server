import React, { useState } from 'react';
import { Key, AlertCircle, LogIn } from 'lucide-react';
import { CaptchaWidget } from '../components/CaptchaWidget';
import { ThemeSwitcher } from '../components/ThemeSwitcher';

interface LoginProps {
  onSuccess: (user: any) => void;
  appName: string;
  ssoEnabled?: boolean;
}

export const Login: React.FC<LoginProps> = ({ onSuccess, appName, ssoEnabled = false }) => {
  const [username, setUsername] = useState<string>('');
  const [password, setPassword] = useState<string>('');
  const [captchaToken, setCaptchaToken] = useState<string>('');
  // A solved challenge is single-use; remounting the widget fetches and solves a fresh one.
  const [captchaRound, setCaptchaRound] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>('');

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const resp = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username,
          password,
          captcha_token: captchaToken || undefined,
        }),
      });

      const data = await resp.json();
      if (!resp.ok) {
        setCaptchaToken('');
        setCaptchaRound((n) => n + 1);
        throw new Error(data.error || 'Login failed');
      }

      if (data.authenticated) {
        onSuccess(data.user);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="ky-auth">
      <div className="ky-auth-theme"><ThemeSwitcher /></div>
      <div className="ky-auth-body">
        <div className="ky-auth-brand">
          <img className="ky-mark" src="/app-icon.png" width={56} height={56} alt="" />
          <h1>{appName || 'KyPulse'}</h1>
          <p>Health and alerts for your Ky apps.</p>
        </div>

        <div className="ky-auth-card">
          {error && <p role="alert" className="ky-auth-error"><AlertCircle size={16} aria-hidden="true" /><span>{error}</span></p>}

          <form onSubmit={handleLogin}>
            <div className="ky-auth-field">
              <label htmlFor="login-username">Username</label>
              <input id="login-username" type="text" autoComplete="username" autoCapitalize="none" spellCheck={false} autoFocus
                value={username} onChange={(e) => setUsername(e.target.value)} required />
            </div>
            <div className="ky-auth-field">
              <label htmlFor="login-password">Password</label>
              <input id="login-password" type="password" autoComplete="current-password"
                value={password} onChange={(e) => setPassword(e.target.value)} required />
            </div>
            <CaptchaWidget key={captchaRound} onToken={setCaptchaToken} />
            {/* An early submit carries no token, fails, and restarts the check from zero. */}
            <button type="submit" className="ky-auth-submit" disabled={loading || !captchaToken}>
              <LogIn aria-hidden="true" />
              <span>{loading ? 'Signing in…' : captchaToken ? 'Sign in' : 'Checking this browser…'}</span>
            </button>
            {ssoEnabled && <>
              <hr className="ky-auth-divider" />
              <a href="/api/sso/kysignon/login" className="ky-auth-sso">
                <Key aria-hidden="true" />
                <span>Continue with single sign-on</span>
              </a>
            </>}
          </form>
        </div>
      </div>
    </main>
  );
};
