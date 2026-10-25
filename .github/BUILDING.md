# 仓库内前端与 Release 构建

根工作流统一使用 `.github/actions/build-frontend`，从当前检出的 `web/` 构建。
Node 使用 24，pnpm 使用 `web/package.json` 中锁定的版本，并执行冻结安装、
前端测试、语言入口生成及 Vite 构建。无需 Crowdin 凭据或下载上游前端发布包。

- PR / 手动测试构建：构建一次标准前端，交给所有后端平台。
- Beta：构建一次标准前端，后端的 WebVersion 读取产物 VERSION。
- Release：分别构建标准版、Lite，保留现有后端多平台矩阵；同时发布独立前端压缩包和 SHA-256 文件。
- Docker：标准版、Lite 分别嵌入对应前端，保留现有镜像变体及平台。

工作流通过 artifact 将前端放到 `public/dist`。`build.sh` 设置
`OPENLIST_FRONTEND_DIR` 时只使用指定目录，不会被上游前端覆盖。
目录必须包含非空的 `index.html`、`VERSION`；无效输入直接失败。
未设置该变量时保留原有下载行为，兼容原先的独立调用方式。

## 本地复现

在仓库根目录执行，先安装 Node 24、Go（版本见 `go.mod`）和 pnpm：

```bash
npm install --global "$(node -p "require('./web/package.json').packageManager")"
(cd web && HUSKY=0 pnpm install --frozen-lockfile && pnpm test)
OPENLIST_FRONTEND_VERSION=v4.2.8-custom bash .github/scripts/build-frontend.sh standard
OPENLIST_FRONTEND_DIR=web/dist OPENLIST_VERSION=v4.2.8-custom bash build.sh release docker
./bin/openlist version
```

`docker` 在此表示构建当前系统的后端程序到 `bin/openlist`，不启动或发布镜像。
前端脚本的参数改成 `lite`，后端参数改成 `release lite docker`，即可构建 Lite。
不指定前端版本时，自动使用 package.json 版本与当前提交短 SHA。

## 提交与验证

前端配置和语言 JSON 是构建源文件。提交本次改动时，需要一并包含新显示的
`web/package.json`、`web/tsconfig.json`、`web/public` JSON 和 `web/src/lang` 文件，
以及工作流、构建脚本和锁文件；生成的 `web/dist`、`public/dist`、依赖及本地产物仍被忽略。

```bash
python3 .github/scripts/test-prepared-frontend.py
bash -n build.sh .github/scripts/build-frontend.sh
actionlint
```

前端现有类型错误与生产构建独立；本次工作流保留测试和生产构建，不把尚未通过的
`pnpm lint` 加成发布门槛。完整的跨平台构建、发布和容器推送需要在 GitHub Actions 中运行。
