import { useEffect, useRef } from 'react';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { api } from '@/lib/api';
import { useAuthStore } from '@/store/auth';

export function AuthCallback() {
  const navigate = useNavigate();
  const search = useSearch({ from: '/auth/callback' });
  const processedRef = useRef(false);

  useEffect(() => {
    if (processedRef.current) return;
    processedRef.current = true;

    const code = (search as Record<string, string>).code;
    if (!code) {
      navigate({ to: '/login' });
      return;
    }

    api
      .getGithubCallbackTokens(code)
      .then((data) => {
        useAuthStore
          .getState()
          .setAuth(data.access_token, data.refresh_token, data.user);
        navigate({ to: '/' });
      })
      .catch((err) => {
        console.error('Auth callback failed:', err);
        navigate({ to: '/login' });
      });
  }, [navigate, search]);

  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-muted-foreground">Signing in...</p>
    </div>
  );
}
