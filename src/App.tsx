import { Suspense, useState } from "react";
import { Routes, Route, Link, useLocation, useNavigate } from "react-router-dom";
import {
  Button,
  Input,
  Navbar,
  NavbarBrand,
  NavbarContent,
  NavbarItem,
  Spinner,
  Tooltip,
} from "@heroui/react";
import Home from "./pages/Home";
import ToolPage from "./pages/ToolPage";
import NotFound from "./pages/NotFound";
import { SearchContext } from "./search";
import { currentTheme, storeTheme, type Theme } from "./utils/theme";

export default function App() {
  const navigate = useNavigate();
  const location = useLocation();
  const [query, setQuery] = useState("");
  const [theme, setTheme] = useState<Theme>(currentTheme);
  const showSearch = location.pathname === "/";

  function toggleTheme() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    storeTheme(next);
    setTheme(next);
  }

  return (
    <SearchContext.Provider value={{ query, setQuery }}>
      <div className="min-h-full bg-default-50">
        <Navbar maxWidth="xl" isBordered isBlurred>
          <NavbarBrand>
            <Link to="/" className="flex items-center gap-2">
              <span className="text-2xl" aria-hidden>
                🧰
              </span>
              <span className="text-lg font-bold text-foreground">NiceTools</span>
            </Link>
          </NavbarBrand>
          <NavbarContent justify="center" className="hidden flex-1 sm:flex">
            {showSearch && (
              <NavbarItem className="w-full max-w-xl">
                <Input
                  aria-label="搜索工具"
                  placeholder="搜索工具名称、说明、分类或标签…"
                  value={query}
                  onValueChange={setQuery}
                  isClearable
                  onClear={() => setQuery("")}
                  variant="bordered"
                  size="sm"
                  startContent={
                    <span className="text-default-400" aria-hidden>
                      🔍
                    </span>
                  }
                  classNames={{
                    base: "w-full",
                  }}
                />
              </NavbarItem>
            )}
          </NavbarContent>
          <NavbarContent justify="end">
            <NavbarItem>
              <Tooltip content="切换深浅色主题">
                <Button
                  variant="light"
                  size="sm"
                  aria-label="切换主题"
                  onPress={toggleTheme}
                  startContent={<span aria-hidden>{theme === "dark" ? "🌙" : "🌞"}</span>}
                >
                  <span className="hidden sm:inline">切换主题</span>
                </Button>
              </Tooltip>
            </NavbarItem>
            <NavbarItem>
              <Button
                variant="light"
                size="sm"
                onPress={() => navigate("/")}
                startContent={<span aria-hidden>🧰</span>}
              >
                全部工具
              </Button>
            </NavbarItem>
          </NavbarContent>
        </Navbar>

        {showSearch && (
          <div className="border-b border-default-200 bg-background px-4 py-3 sm:hidden">
            <Input
              aria-label="搜索工具"
              placeholder="搜索工具名称、说明、分类或标签…"
              value={query}
              onValueChange={setQuery}
              isClearable
              onClear={() => setQuery("")}
              variant="bordered"
              size="md"
              startContent={
                <span className="text-default-400" aria-hidden>
                  🔍
                </span>
              }
            />
          </div>
        )}

        <main className="mx-auto w-full max-w-6xl min-w-0 overflow-x-hidden px-4 py-8 sm:px-6">
          <Suspense
            fallback={
              <div className="flex justify-center py-24">
                <Spinner label="加载中…" />
              </div>
            }
          >
            <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/tools/:id" element={<ToolPage />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </Suspense>
        </main>

        <footer className="mx-auto w-full max-w-6xl px-4 pb-10 pt-4 text-center text-xs text-default-400 sm:px-6">
          NiceTools · 安全快捷，注重隐私保护
        </footer>
      </div>
    </SearchContext.Provider>
  );
}
