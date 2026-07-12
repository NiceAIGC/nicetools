import { heroui } from "@heroui/react";

// Tailwind v4 CSS-first 配置需要一个默认导出的 Tailwind 插件对象；
// @heroui/react 导出的 heroui() 是一个返回插件的工厂函数，这里做桥接。
export default heroui();
