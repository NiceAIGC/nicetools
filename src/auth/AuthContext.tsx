import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { Button, Spinner } from "@heroui/react";
import { api, type Session, type ToolAccess } from "../api/client";

interface AuthValue {
  session: Session | null;
  loading: boolean;
  error: string;
  tools: ToolAccess[] | null;
  refresh: () => Promise<void>;
  refreshTools: () => Promise<void>;
}
const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [tools, setTools] = useState<ToolAccess[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const flight = useRef<Promise<void> | null>(null);
  const refresh = useCallback((): Promise<void> => {
    if (flight.current) return flight.current;
    const pending = (async () => {
      try {
        const value = await api<Session>("/session");
        const access = await api<ToolAccess[]>("/tools");
        setSession(value);
        setTools(access);
        setError("");
      } catch (e) {
        setSession(null);
        setTools(null);
        setError(e instanceof Error ? e.message : "无法连接服务");
      } finally {
        setLoading(false);
        flight.current = null;
      }
    })();
    flight.current = pending;
    return pending;
  }, []);

  useEffect(() => {
    void refresh();
    const check = () => { void refresh(); };
    window.addEventListener("focus", check);
    const timer = window.setInterval(check, 60000);
    return () => { window.removeEventListener("focus", check); window.clearInterval(timer); };
  }, [refresh]);

  const value = useMemo(() => ({ session, tools, loading, error, refresh, refreshTools: refresh }), [session, tools, loading, error, refresh]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("AuthProvider missing");
  return value;
}
export function RequireAuth({ children, admin = false }: { children: ReactNode; admin?: boolean }) {
  const { session, loading, error, refresh } = useAuth();
  const location = useLocation();
  if (loading) return <Spinner label="加载登录状态…" />;
  if (error) return <div className="space-y-3"><p role="alert" className="text-danger">{error}</p><Button onPress={() => void refresh()}>重试</Button></div>;
  if (!session?.user) return <Navigate replace to={`/login?next=${encodeURIComponent(location.pathname)}`} />;
  if (admin && session.user.role !== "admin") return <p role="alert">需要管理员权限，请联系管理员。</p>;
  return children;
}
