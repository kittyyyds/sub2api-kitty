
import { KITTY_MODE } from "@/lib/kitty";
import { FileText, ImagePlus, Images, Maximize2, Music2, Video } from "lucide-react";

const allNavigationTools = [
    {
        slug: "canvas",
        label: "我的画布",
        icon: Maximize2,
    },
    {
        slug: "image",
        label: "生图工作台",
        icon: ImagePlus,
    },
    {
        slug: "video",
        label: "视频创作台",
        icon: Video,
    },
    {
        slug: "music",
        label: "音乐创作台",
        icon: Music2,
    },
    {
        slug: "prompts",
        label: "提示词库",
        icon: FileText,
    },
    {
        slug: "assets",
        label: "我的素材",
        icon: Images,
    },
] as const;

export const navigationTools = KITTY_MODE ? allNavigationTools.filter((tool) => ["canvas", "assets"].includes(tool.slug)) : allNavigationTools;
export type NavigationToolSlug = (typeof allNavigationTools)[number]["slug"];
