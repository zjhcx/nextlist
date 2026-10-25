import { currentLang } from "~/app/i18n"

const messages: Record<string, Record<string, string>> = {
  en: {
    sessions: "Login sessions",
    sessions_refresh: "Refresh",
    sessions_logout_others: "Sign out other clients",
    sessions_empty: "No active sessions",
    sessions_unknown: "Unknown client",
    sessions_current: "Current client",
    sessions_created: "Signed in",
    sessions_active: "Last active",
    sessions_expires: "Expires",
    sessions_logout: "Sign out",
    sessions_confirm: "Sign out the selected client(s)?",
  },
  "zh-CN": {
    sessions: "登录客户端",
    sessions_refresh: "刷新",
    sessions_logout_others: "退出其他客户端",
    sessions_empty: "暂无登录客户端",
    sessions_unknown: "未知客户端",
    sessions_current: "当前客户端",
    sessions_created: "登录时间",
    sessions_active: "最近活动",
    sessions_expires: "过期时间",
    sessions_logout: "退出登录",
    sessions_confirm: "确定退出所选客户端吗？",
  },
  "zh-TW": {
    sessions: "登入用戶端",
    sessions_refresh: "重新整理",
    sessions_logout_others: "登出其他用戶端",
    sessions_empty: "暫無登入用戶端",
    sessions_unknown: "未知用戶端",
    sessions_current: "目前用戶端",
    sessions_created: "登入時間",
    sessions_active: "最近活動",
    sessions_expires: "到期時間",
    sessions_logout: "登出",
    sessions_confirm: "確定登出所選用戶端嗎？",
  },
}

export const sessionText = (key: string) =>
  messages[currentLang()]?.[key] ?? messages.en[key]
