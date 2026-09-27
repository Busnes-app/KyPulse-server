import React, { useCallback, useEffect, useState } from 'react';
import { AppHeader } from './components/AppHeader';
import { AlertBar } from './components/AlertBar';
import { Status } from './pages/Status';
import { Alerts } from './pages/Alerts';
import { AppDetail } from './pages/AppDetail';
import { Login } from './pages/Login';
import { ChangePassword } from './pages/ChangePassword';
import { Backup } from './pages/Backup';
import { Settings } from './pages/Settings';
import './styles/theme.css';
import './ky-ui/tokens.css';
import './ky-ui/navigation.css';
import { secureFetch } from './api';
import { navigate, useHashRoute } from './router';
import { ApiError, getStatus, type StatusSummary } from './monitor';

// statusEvery is how often the alert bar re-reads /api/status.
const statusEvery = 15_000;

export const App: React.FC = () => {
  const [user, setUser] = useState<any>(null);
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState<boolean>(true);
  const [settings, setSettings] = useState<any>(null);
  const [status, setStatus] = useState<StatusSummary | null>(null);
  const [statusLoading, setStatusLoading] = useState(false);
  const route = useHashRoute();

  useEffect(() => {
    const checkAuth = async () => {
      try {
        const [authResp, setResp] = [await fetch('/api/auth/me'), await fetch('/api/settings')];
        if (setResp.ok) setSettings(await setResp.json());
        if (authResp.ok) {
          const a = await authResp.json();
          if (a.authenticated) setUser(a.user);
        }
      } catch (err) {
        console.error('Initialization error:', err);
      } finally {
        setLoading(false);
      }
    };
    checkAuth();
  }, []);

  // /api/settings returns more fields once authenticated, so re-read it after login.
  const loadSettings = async () => {
    const resp = await fetch('/api/settings');
    if (resp.ok) setSettings(await resp.json());
  };

  const refreshStatus = useCallback(async () => {
    setStatusLoading(true);
    try {
      setStatus(await getStatus());
    } catch (err) {
      // A 401 means the session ended: back to the login screen, no stale data behind it.
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
        setNotice('Signed out. Sign in again.');
      }
      setStatus(null);
    } finally {
      setStatusLoading(false);
    }
  }, []);

  const signedIn = !!user && !user.must_change_password;
  useEffect(() => {
    if (!signedIn) return;
    void refreshStatus();
    const timer = window.setInterval(() => void refreshStatus(), statusEvery);
    return () => window.clearInterval(timer);
  }, [signedIn, refreshStatus]);

  // A viewer typing #/backup lands on Status; unknown paths do too.
  const isAdmin = user?.role === 'admin';
  useEffect(() => {
    if (!signedIn) return;
    const known = ['status', 'alerts', 'apps', 'settings', 'backup'];
    const head = route.parts[0] ?? 'status';
    if (!known.includes(head) || (head === 'backup' && !isAdmin) || (head === 'apps' && route.parts.length !== 2)) navigate('/status');
  }, [signedIn, isAdmin, route]);

  const handleLogout = async () => {
    await secureFetch('/api/auth/logout', { method: 'POST' });
    setUser(null);
    setStatus(null);
    navigate('/status');
  };

  if (loading) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg)', color: 'var(--ink)' }}>
        Loading {settings?.app_name || 'kyPulse'}...
      </div>
    );
  }

  if (!user) {
    return (
      <>
      {notice && <p role="status" style={{ padding: 16 }}>{notice}</p>}
      <Login
        appName={settings?.app_name || 'kyPulse'}
        onSuccess={(u) => {
          setNotice('');
          setUser(u);
          void loadSettings();
        }}
      />
      </>
    );
  }

  if (user.must_change_password) {
    return <ChangePassword onLogout={handleLogout} onComplete={() => {
      setUser(null);
      setNotice('Password changed. Sign in with your new password.');
    }} />;
  }

  const head = route.parts[0] ?? 'status';
  return (
    <div className="app-shell">
      <AppHeader
        appName={settings?.app_name || 'kyPulse'}
        activePath={route.path}
        user={user}
        onLogout={handleLogout}
      />

      <main className="app-main">
        <AlertBar status={status} loading={statusLoading} isAdmin={isAdmin} />
        {head === 'status' && <Status user={user} onChanged={refreshStatus} />}
        {head === 'alerts' && <Alerts user={user} />}
        {head === 'apps' && route.parts[1] && <AppDetail key={route.parts[1]} id={route.parts[1]} user={user} onChanged={refreshStatus} />}
        {head === 'backup' && isAdmin && <Backup />}
        {head === 'settings' && <Settings settings={settings} user={user} onChanged={refreshStatus} />}
      </main>
    </div>
  );
};
