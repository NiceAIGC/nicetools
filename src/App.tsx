import { Suspense, useState } from "react";
import { Routes, Route, Link, useLocation, useNavigate } from "react-router-dom";
import {
  Input,
  Navbar,
  NavbarBrand,
  NavbarContent,
  NavbarItem,
  Spinner,
} from "@heroui/react";
import Home from "./pages/Home";
import ToolPage from "./pages/ToolPage";
import NotFound from "./pages/NotFound";
import { SearchContext } from "./search";

export default function App() {
  const navigate = useNavigate();
  const location = useLocation();
  const [query, setQuery] = useState("");
  const showSearch = location.pathname === "/";

  return (
    <SearchContext.Provider value={{ query, setQuery }}>
      <div className="min-h-full bg-default-50">
        <Navbar maxWidth="xl" isBordered className="bg-background">
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
                  placeholder="搜索工具名称、说明、分组或分类…"
                  value={query}
                  onValueChange={setQuery}
                  isClearable
                  onClear={() => setQuery("")}
                  variant="bordered"
                  size="sm"
                  classNames={{
                    base: "w-full",
                    inputWrapper: "bg-default-50",
                  }}
                />
              </NavbarItem>
            )}
          </NavbarContent>
          <NavbarContent justify="end">
            <NavbarItem>
              <button
                type="button"
                onClick={() => navigate("/")}
                className="text-sm text-default-500 transition-colors hover:text-foreground"
              >
                全部工具
              </button>
            </NavbarItem>
          </NavbarContent>
        </Navbar>

        {showSearch && (
          <div className="border-b border-default-200 bg-background px-4 py-3 sm:hidden">
            <Input
              aria-label="搜索工具"
              placeholder="搜索工具名称、说明、分组或分类…"
              value={query}
              onValueChange={setQuery}
              isClearable
              onClear={() => setQuery("")}
              variant="bordered"
              size="md"
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
          NiceTools · 纯前端运行，数据仅保存在本地浏览器
        </footer>
      </div>
    </SearchContext.Provider>
  );
}
