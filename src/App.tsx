import { Suspense, useState } from "react";
import { Routes, Route, Link, useLocation, useNavigate } from "react-router-dom";
import { Button, Input, Navbar, NavbarBrand, NavbarContent, NavbarItem, Spinner, Tooltip } from "@heroui/react";
import Home from "./pages/Home";
import ToolPage from "./pages/ToolPage";
import NotFound from "./pages/NotFound";
import Login from "./pages/Login";
import Account from "./pages/Account";
import Admin from "./pages/Admin";
import { SearchContext } from "./search";
import { currentTheme, storeTheme, type Theme } from "./utils/theme";
import { RequireAuth, useAuth } from "./auth/AuthContext";
import { api, json } from "./api/client";

export default function App() {
  const { session, loading, refresh } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [query, setQuery] = useState("");
  const [theme, setTheme] = useState<Theme>(currentTheme);
  const [logoutBusy, setLogoutBusy] = useState(false);
  const [error, setError] = useState("");
  const showSearch = location.pathname === "/";
  const user = session?.user;
  function toggleTheme() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    storeTheme(next); setTheme(next);
  }
  async function logout() {
    setLogoutBusy(true); setError("");
    try { await api("/auth/logout", json("POST", {})); await refresh(); navigate("/"); }
    catch (e) { setError(e instanceof Error ? e.message : "退出失败"); }
    finally { setLogoutBusy(false); }
  }
  return <SearchContext.Provider value={{ query, setQuery }}>
    <div className="min-h-full bg-default-50">
      <Navbar maxWidth="xl" isBordered isBlurred classNames={{ wrapper: "gap-2 px-3 sm:px-6" }}>
        <NavbarBrand className="flex-grow-0">
          <Link to="/" className="font-bold text-foreground">NiceTools</Link>
        </NavbarBrand>
        <NavbarContent justify="center" className="hidden flex-1 lg:flex">
          {showSearch && <NavbarItem className="w-full max-w-xl"><Input aria-label="搜索工具" placeholder="搜索名称、分类或标签…" value={query} onValueChange={setQuery} isClearable onClear={() => setQuery("")} variant="bordered" size="sm" /></NavbarItem>}
        </NavbarContent>
        <NavbarContent justify="end" className="gap-1 sm:gap-2">
          <NavbarItem><Tooltip content="切换深浅色主题"><Button variant="light" size="sm" aria-label="切换主题" onPress={toggleTheme}>{theme === "dark" ? "浅色" : "深色"}</Button></Tooltip></NavbarItem>
          <NavbarItem className="hidden sm:block"><Button variant="light" size="sm" onPress={() => navigate("/")}>全部工具</Button></NavbarItem>
          {user?.role === "admin" && <NavbarItem><Button size="sm" variant="flat" color="primary" onPress={() => navigate("/admin")}>管理后台</Button></NavbarItem>}
          {user ? <>
            <NavbarItem><Button size="sm" variant="light" onPress={() => navigate("/account")} className="max-w-24 truncate">{user.display_name || user.username}</Button></NavbarItem>
            <NavbarItem><Button size="sm" variant="light" isLoading={logoutBusy} onPress={() => void logout()}>退出</Button></NavbarItem>
          </> : <NavbarItem><Button size="sm" color="primary" isDisabled={loading} onPress={() => navigate("/login")}>登录</Button></NavbarItem>}
        </NavbarContent>
      </Navbar>
      {showSearch && <div className="border-b border-default-200 bg-background px-4 py-3 lg:hidden"><Input aria-label="搜索工具" placeholder="搜索工具名称、分类或标签…" value={query} onValueChange={setQuery} isClearable onClear={() => setQuery("")} variant="bordered" /></div>}
      <main className="mx-auto w-full max-w-6xl min-w-0 overflow-x-hidden px-4 py-8 sm:px-6">
        {error && <p role="alert" className="mb-4 text-danger">{error}</p>}
        <Suspense fallback={<div className="flex justify-center py-24"><Spinner label="加载中…" /></div>}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/tools/:id" element={<ToolPage />} />
            <Route path="/login" element={<Login />} />
            <Route path="/account" element={<RequireAuth><Account /></RequireAuth>} />
            <Route path="/admin" element={<RequireAuth admin><Admin /></RequireAuth>} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </Suspense>
      </main>
      <footer className="mx-auto max-w-6xl px-4 pb-10 pt-4 text-center text-xs text-default-400">NiceTools · 本地工具处理，账号与权限由服务器管理</footer>
    </div>
  </SearchContext.Provider>;
}
