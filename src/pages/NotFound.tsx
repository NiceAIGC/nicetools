import { Link } from "react-router-dom";
import { Button } from "@heroui/react";

export default function NotFound() {
  return (
    <div className="flex flex-col items-center gap-4 py-24 text-center">
      <span className="text-5xl" aria-hidden>
        🧭
      </span>
      <h1 className="text-xl font-bold text-foreground">页面不存在</h1>
      <p className="text-sm text-default-500">你访问的工具可能已移动或尚未上线。</p>
      <Button as={Link} to="/" color="primary" variant="flat">
        返回工具集
      </Button>
    </div>
  );
}
