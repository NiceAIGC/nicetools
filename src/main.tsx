import React from "react";
import ReactDOM from "react-dom/client";
import { HeroUIProvider } from "@heroui/react";
import { HashRouter } from "react-router-dom";
import App from "./App";
import "./index.css";

// 使用 HashRouter，静态托管（含 GitHub Pages）无需服务端路由配置即可深链。
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <HeroUIProvider>
      <HashRouter>
        <App />
      </HashRouter>
    </HeroUIProvider>
  </React.StrictMode>,
);
