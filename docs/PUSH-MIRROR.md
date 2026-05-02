# 仓库镜像策略

本仓库 **同时维护两个远端**：

| 站点      | 角色              | URL                                                |
|-----------|-------------------|----------------------------------------------------|
| GitHub    | 主站 / fetch 源   | `https://github.com/z1677930848/lingcdn.git`       |
| GitCode   | 国内镜像 / 只推   | `https://gitcode.com/z1677930848/lingcdn.git`      |

两个站必须始终保持同步。

## 一条命令双推

`origin` 已经配好了两条 push URL，所以**无需任何额外操作**，直接：

```bash
git push origin main
```

就会自动推到 GitHub 和 GitCode 两个站。

查看 / 确认配置：

```bash
git remote -v
# origin  https://github.com/z1677930848/lingcdn.git (fetch)
# origin  https://github.com/z1677930848/lingcdn.git (push)
# origin  https://gitcode.com/z1677930848/lingcdn.git (push)
```

## 如果不小心把双推配置弄丢了

按下面两条命令重建：

```bash
git remote set-url --add --push origin https://github.com/z1677930848/lingcdn.git
git remote set-url --add --push origin https://gitcode.com/z1677930848/lingcdn.git
```

## 鉴权

- **GitHub**：首次 push 时浏览器登录 / GCM 弹窗，之后 credential manager 自动缓存。
- **GitCode**：**不再接受密码**，必须用 PAT。
  1. 打开 <https://gitcode.com/-/profile/personal_access_tokens>
  2. 新建 token，权限至少勾 `write_repository`
  3. 首次 push 时 GCM 弹窗里用用户名 + **PAT 作为密码**
  4. 成功一次后会自动缓存

## 推送/发布规约

- **任何 `git push` / `force-push` / tag 推送都必须同步到两站**；不允许临时 `git push github-only` 之类的绕过。
- 如果 GitCode 端鉴权失败导致某次推送只成功了 GitHub 一半，**必须当场修好 PAT 并补推 GitCode**。
- 发布新版本时打的 `v*` tag 也要同推（`git push origin v1.0.8` 会自动双推）。

## 为什么用"单 origin 双 push URL"而不是"两个 remote"

- **方案 A**（当前）：只有一个 `origin`，有 2 条 push URL —— 一条命令必然双推，物理上无法只推一半。
- **方案 B**（被拒绝）：`origin = github`, `gitcode = gitcode`，每次要执行 `git push origin main && git push gitcode main` —— 只要有一次手滑只执行第一条，两边就分叉，久而久之变成两个仓库。

选 A 是为了**默认就正确**。
