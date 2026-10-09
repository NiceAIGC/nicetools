import { Suspense, useState } from "react";
import { Routes, Route, Link, useLocation, useNavigate } from "react-router-dom";
import { Avatar, Button, Dropdown, DropdownItem, DropdownMenu, DropdownTrigger, Input, Navbar, NavbarBrand, NavbarContent, NavbarItem, NavbarMenu, NavbarMenuItem, NavbarMenuToggle, Spinner, Tooltip } from "@heroui/react";
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
  const [menuOpen, setMenuOpen] = useState(false);
  const showSearch = location.pathname === "/";
  const user = session?.user;
  const displayName = user?.display_name || user?.username || "账户";
  const initials = displayName.slice(0, 1).toUpperCase();

  function toggleTheme() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    storeTheme(next);
    setTheme(next);
  }

  async function logout() {
    setLogoutBusy(true);
    setError("");
    try {
      await api("/auth/logout", json("POST", {}));
      await refresh();
      navigate("/");
      setMenuOpen(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : "退出失败");
    } finally {
      setLogoutBusy(false);
    }
  }

  function go(path: string) {
    navigate(path);
    setMenuOpen(false);
  }

  return <SearchContext.Provider value={{ query, setQuery }}>
    <div className="flex min-h-screen flex-col bg-default-50">
      <Navbar
        maxWidth="xl"
        isBordered
        isBlurred
        isMenuOpen={menuOpen}
        onMenuOpenChange={setMenuOpen}
        classNames={{ base: "bg-background/90", wrapper: "max-w-6xl gap-3 px-4 sm:gap-6 sm:px-6" }}
      >
        <NavbarBrand className="!flex-none">
          <Link to="/" className="inline-flex shrink-0 items-center gap-2 whitespace-nowrap" onClick={() => setMenuOpen(false)}>
            <span aria-hidden="true" className="shrink-0 text-2xl leading-none">🧰</span>
            <span className="text-lg font-semibold tracking-tight text-foreground">NiceTools</span>
          </Link>
        </NavbarBrand>

        <NavbarContent justify="center" className={showSearch ? "hidden min-w-0 flex-1 lg:flex" : "hidden flex-1 md:flex"}>
          {showSearch && <NavbarItem className="w-full max-w-xl">
            <Input
              aria-label="搜索工具"
              placeholder="搜索工具、分类或标签"
              value={query}
              onValueChange={setQuery}
              isClearable
              onClear={() => setQuery("")}
              variant="flat"
              size="sm"
              radius="full"
              startContent={<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="shrink-0 text-default-400"><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" /></svg>}
              classNames={{ inputWrapper: "bg-default-100", input: "text-sm" }}
            />
          </NavbarItem>}
        </NavbarContent>

        <NavbarContent justify="end" className="ml-auto !flex-none gap-2 sm:gap-3">
          <NavbarItem className="hidden md:flex">
            <Button variant={showSearch ? "flat" : "light"} color={showSearch ? "primary" : "default"} radius="full" size="sm" onPress={() => go("/")}>全部工具</Button>
          </NavbarItem>
          {user?.role === "admin" && <NavbarItem className="hidden md:flex">
            <Button variant={location.pathname === "/admin" ? "flat" : "light"} color={location.pathname === "/admin" ? "primary" : "default"} radius="full" size="sm" onPress={() => go("/admin")}>管理后台</Button>
          </NavbarItem>}
          <NavbarItem>
            <Tooltip content={theme === "dark" ? "切换浅色模式" : "切换深色模式"}>
              <Button variant="light" radius="full" isIconOnly size="sm" aria-label={theme === "dark" ? "切换浅色模式" : "切换深色模式"} onPress={toggleTheme}>
                <svg aria-hidden="true" width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
                  {theme === "dark" ? <><circle cx="12" cy="12" r="4" /><path d="M12 2v2m0 16v2M2 12h2m16 0h2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42" /></> : <path d="M20.7 13.2A9 9 0 0 1 10.8 3.3a9 9 0 1 0 9.9 9.9Z" />}
                </svg>
              </Button>
            </Tooltip>
          </NavbarItem>
          {user ? <NavbarItem>
            <Dropdown placement="bottom-end">
              <DropdownTrigger>
                <Button variant="light" radius="full" className="h-10 min-w-10 gap-2 px-1 xl:px-2" aria-label={`打开${displayName}账户菜单`}>
                  <Avatar name={initials} size="sm" color="primary" className="shrink-0" />
                  <span className="hidden max-w-28 truncate text-sm xl:inline">{displayName}</span>
                </Button>
              </DropdownTrigger>
              <DropdownMenu aria-label="账户菜单" onAction={key => { if (key === "account") go("/account"); if (key === "logout") void logout(); }}>
                <DropdownItem key="profile" isReadOnly textValue={displayName} className="opacity-100" description={user.role === "admin" ? "管理员" : "普通用户"} showDivider>{displayName}</DropdownItem>
                <DropdownItem key="account">账号设置</DropdownItem>
                <DropdownItem key="logout" color="danger" className="text-danger" isDisabled={logoutBusy}>退出登录</DropdownItem>
              </DropdownMenu>
            </Dropdown>
          </NavbarItem> : <NavbarItem><Button size="sm" radius="full" color="primary" isDisabled={loading} onPress={() => go("/login")}>登录</Button></NavbarItem>}
          <NavbarMenuToggle aria-label={menuOpen ? "关闭导航菜单" : "打开导航菜单"} className="md:hidden" />
        </NavbarContent>

        <NavbarMenu className="gap-2 border-t border-default-200 bg-background px-4 pb-6 pt-4">
          <NavbarMenuItem><Button variant="flat" className="w-full justify-start" onPress={() => go("/")}>全部工具</Button></NavbarMenuItem>
          {user?.role === "admin" && <NavbarMenuItem><Button variant="flat" color="primary" className="w-full justify-start" onPress={() => go("/admin")}>管理后台</Button></NavbarMenuItem>}
        </NavbarMenu>
      </Navbar>

      {showSearch && <div className="border-b border-default-200 bg-background px-4 py-3 lg:hidden"><div className="mx-auto max-w-6xl"><Input aria-label="搜索工具" placeholder="搜索工具、分类或标签" value={query} onValueChange={setQuery} isClearable onClear={() => setQuery("")} variant="flat" radius="full" startContent={<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="shrink-0 text-default-400"><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" /></svg>} /></div></div>}
      <main className="mx-auto w-full max-w-6xl min-w-0 flex-1 overflow-x-hidden px-4 py-8 sm:px-6">
        {error && <p role="alert" className="mb-4 rounded-medium bg-danger-50 px-4 py-3 text-sm text-danger">{error}</p>}
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
      <footer className="mx-auto max-w-6xl px-4 pb-10 pt-4 text-center text-xs text-default-400">NiceTools · 多种工具处理，安全可靠</footer>
    </div>
  </SearchContext.Provider>;
}
