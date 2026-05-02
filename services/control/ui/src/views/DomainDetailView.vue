<template>
  <div class="page">
    <div v-if="loading" class="loading-wrap">
      <t-loading />
    </div>

    <template v-else-if="domain">
      <!-- 页面 hero：渐变品牌底 + 域名 + CNAME 操作 + 健康摘要 chip。
           把高频操作（复制 CNAME / 重新生成）和"看一眼就懂"的健康状态
           上提到顶部，避免用户为了校验状态在 7 个 tab 里来回跳。 -->
      <section class="hero">
        <div class="hero-aurora" aria-hidden="true"></div>
        <div class="hero-grid" aria-hidden="true"></div>
        <div class="hero-content">
          <div class="hero-top">
            <button class="hero-back" type="button" @click="goBack">
              <ArrowLeft :size="16" />
              <span>返回域名列表</span>
            </button>
            <div class="hero-status">
              <t-tag
                shape="round"
                :theme="domain.enabled ? 'success' : 'default'"
                variant="light"
                size="medium"
              >
                <template #icon>
                  <CheckCircle2 v-if="domain.enabled" :size="14" />
                  <CircleSlash v-else :size="14" />
                </template>
                {{ domain.enabled ? '正常运行' : '已停用' }}
              </t-tag>
            </div>
          </div>

          <div class="hero-main">
            <div class="hero-icon">
              <Globe2 :size="30" />
            </div>
            <div class="hero-title-wrap">
              <div class="hero-eyebrow">域名详情</div>
              <h1 class="hero-title">{{ domain.name }}</h1>
              <div class="hero-cname">
                <code>{{ domain.cname || '尚未生成 CNAME' }}</code>
                <button
                  v-if="domain.cname"
                  class="hero-icon-btn"
                  type="button"
                  title="复制 CNAME"
                  @click="copyCname"
                >
                  <Copy :size="14" />
                </button>
                <button
                  class="hero-icon-btn"
                  type="button"
                  title="重新生成 CNAME"
                  :disabled="saving === 'regen-cname'"
                  @click="regenCname"
                >
                  <RefreshCw :size="14" :class="{ 'spin': saving === 'regen-cname' }" />
                </button>
              </div>
            </div>
          </div>

          <div class="hero-chips">
            <div
              v-for="chip in healthChips"
              :key="chip.key"
              :class="['hchip', chip.on ? 'hchip-on' : 'hchip-off']"
            >
              <component :is="chip.icon" :size="14" />
              <span>{{ chip.label }}</span>
              <span class="hchip-state">{{ chip.on ? '已启用' : '已关闭' }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- 基本信息：用 TDescriptions 替换原来手写的 info-grid，
           标签/值对齐更自然，bordered 模式与下方配置卡的视觉层级一致。 -->
      <t-card class="info-card" :bordered="true" :body-style="{ padding: '0' }">
        <template #header>
          <div class="info-card-header">
            <Sparkles :size="16" class="info-card-icon" />
            <span>基本信息</span>
          </div>
        </template>
        <t-descriptions
          :column="2"
          layout="horizontal"
          bordered
          size="medium"
          class="info-desc"
        >
          <t-descriptions-item label="CNAME">
            <code class="cn-code">{{ domain.cname || '-' }}</code>
          </t-descriptions-item>
          <t-descriptions-item label="套餐">{{ clusterName || '-' }}</t-descriptions-item>
          <t-descriptions-item label="源站">{{ originSummary || '-' }}</t-descriptions-item>
          <t-descriptions-item label="创建时间">
            <span :title="formatTimeFull(domain.created_at)">{{ formatTime(domain.created_at) }}</span>
          </t-descriptions-item>
          <t-descriptions-item label="更新时间">
            <span :title="formatTimeFull(domain.updated_at)">{{ formatTime(domain.updated_at) }}</span>
          </t-descriptions-item>
        </t-descriptions>
      </t-card>

      <!-- 配置分区（对标截图顶部 tabs） -->
      <t-card class="section-card" bordered>
        <t-tabs v-model="activeTab" class="admin-tabs domain-detail-tabs">
          <!-- 基本配置 — renamed inner section to "调度与域名" to avoid
               the near-dup "基本配置 → 基本设置" that made scanning noisy. -->
          <t-tab-panel value="basic">
            <template #label>
              <span class="tab-label"><Server :size="14" /><span>基本配置</span></span>
            </template>
            <div class="tab-body">
              <!-- Section: 调度与域名 — cluster / domain -->
              <section class="config-section">
                <h3 class="section-title">调度与域名</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">套餐（集群）</label>
                    <div class="sec-value">
                      <t-select v-model="form.line_group_id" :options="clusterOptions" placeholder="请选择" class="fld-default" />
                      <div class="sec-hint">切换套餐/集群只影响新请求的调度节点，不会改动 CNAME。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">域名</label>
                    <div class="sec-value">
                      <t-input v-model="form.name" class="fld-default" />
                      <div class="sec-hint">修改后 CNAME 可能重新生成，如需多个域名共用同一组回源请使用多条记录。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">网站启停</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.enabled" size="large" />
                        <span class="switch-text">{{ form.enabled ? '已启用' : '已停用' }}</span>
                      </div>
                      <div class="sec-hint">关闭后节点将拒绝本域名的所有请求。</div>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">HTTP 设置</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">监听端口</label>
                    <div class="sec-value">
                      <div class="port-wrap">
                        <t-input v-model="listenPortText" type="number" class="fld-port" placeholder="0 (默认)" />
                        <t-button variant="outline" size="small" @click="setListenPort(80)">80</t-button>
                        <t-button variant="outline" size="small" @click="setListenPort(443)">443</t-button>
                        <t-button variant="outline" size="small" @click="setListenPort(8080)">8080</t-button>
                        <t-button variant="text" size="small" @click="setListenPort(0)">默认</t-button>
                      </div>
                      <div class="sec-hint">留空或 0 使用默认端口 80；如需关闭明文 HTTP，请在「HTTPS 设置」中强制启用 HTTPS。</div>
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <t-button theme="primary" :loading="saving === 'basic'" :disabled="!basicDirty" @click="saveBasic">保存基本配置</t-button>
                <t-button variant="outline" :disabled="!basicDirty || saving === 'basic'" @click="resetBasic">重置</t-button>
                <span v-if="basicDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </t-tab-panel>

          <!-- 回源设置 -->
          <t-tab-panel value="origin">
            <template #label>
              <span class="tab-label"><CloudUpload :size="14" /><span>回源设置</span></span>
            </template>
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">源站</h3>
                <div class="sec-form">
                  <div class="sec-row sec-row--top">
                    <label class="sec-label">回源地址</label>
                    <div class="sec-value">
                      <div class="origin-list">
                        <div class="origin-list-head">
                          <span class="origin-col origin-col-addr">地址</span>
                          <span class="origin-col origin-col-weight">权重</span>
                          <span class="origin-col origin-col-enabled">启用</span>
                          <span class="origin-col origin-col-action"></span>
                        </div>
                        <div
                          v-for="(entry, idx) in domainOriginsForm"
                          :key="idx"
                          class="origin-row"
                        >
                          <div class="origin-handle" aria-hidden="true">
                            <GripVertical :size="14" />
                          </div>
                          <t-input
                            v-model="entry.address"
                            placeholder="1.2.3.4 或 origin.example.com:8080"
                            class="origin-address"
                          />
                          <div class="origin-weight">
                            <t-input-number
                              v-model="entry.weight"
                              :min="1"
                              :max="100"
                              class="fld-status"
                            />
                          </div>
                          <div class="origin-enabled">
                            <t-switch v-model="entry.enabled" size="small" />
                            <span class="switch-hint">{{ entry.enabled ? "启用" : "停用" }}</span>
                          </div>
                          <button
                            class="origin-del"
                            type="button"
                            :disabled="domainOriginsForm.length <= 1"
                            title="删除"
                            @click="removeDomainOrigin(idx)"
                          >
                            <Trash2 :size="14" />
                          </button>
                        </div>
                        <button class="origin-add" type="button" @click="addDomainOrigin">
                          <Plus :size="14" />
                          <span>添加源站</span>
                        </button>
                      </div>
                      <div class="sec-hint">地址可填 IP 或主机名，端口可选；不填端口时按上方"回源端口"兜底。权重 1-100 控制流量占比，关闭某条会使其完全不参与回源。</div>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">负载方式</label>
                    <div class="sec-value">
                      <t-radio-group v-model="form.load_balance_method">
                        <t-radio value="round_robin">轮循</t-radio>
                        <t-radio value="ip_hash">定源</t-radio>
                      </t-radio-group>
                      <div class="sec-hint">当添加多个源站时，负载方式为轮循时，请求平均地转发到各个源站，当为 IP Hash 时，同一个用户的请求固定发往一个源站，一般用于会话保持。</div>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">源站健康检查</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.origin_health_check!.enabled" />
                        <span class="switch-text">{{ form.origin_health_check?.enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">添加多个源站时，开启此检查后，节点会定期探测每个源站的健康状态：连续探测失败时自动从可选池中下线，恢复后再重新上线。当所有源站都不健康时会回退到全部源站，以避免出现"无源站可选"的故障。</div>
                    </div>
                  </div>

                  <template v-if="form.origin_health_check?.enabled">
                    <div class="sec-row">
                      <label class="sec-label">探测间隔</label>
                      <div class="sec-value">
                        <div class="timeout-wrap">
                          <t-input-number
                            v-model="form.origin_health_check!.interval_sec"
                            :min="5"
                            :max="600"
                            class="fld-timeout"
                          />
                          <span class="unit-suffix">秒</span>
                        </div>
                        <div class="sec-hint">两次探测之间的间隔，最小 5 秒，建议 10-60 秒。</div>
                      </div>
                    </div>
                    <div class="sec-row">
                      <label class="sec-label">探测路径</label>
                      <div class="sec-value">
                        <t-input v-model="form.origin_health_check!.path" placeholder="/" class="fld-default" />
                        <div class="sec-hint">节点会向 <code>http://源站地址{{ form.origin_health_check?.path || '/' }}</code> 发送 GET 请求。建议使用源站上的轻量级健康检查端点（例如 <code>/healthz</code>）。</div>
                      </div>
                    </div>
                    <div class="sec-row">
                      <label class="sec-label">超时时间</label>
                      <div class="sec-value">
                        <div class="timeout-wrap">
                          <t-input-number
                            v-model="form.origin_health_check!.timeout_ms"
                            :min="100"
                            :max="60000"
                            :step="100"
                            class="fld-timeout"
                          />
                          <span class="unit-suffix">毫秒</span>
                        </div>
                        <div class="sec-hint">单次探测超时；超出视为本次探测失败。</div>
                      </div>
                    </div>
                    <div class="sec-row">
                      <label class="sec-label">失败下线阈值</label>
                      <div class="sec-value">
                        <div class="timeout-wrap">
                          <t-input-number
                            v-model="form.origin_health_check!.fail_threshold"
                            :min="1"
                            :max="20"
                            class="fld-timeout"
                          />
                          <span class="unit-suffix">次</span>
                        </div>
                        <div class="sec-hint">连续探测失败达到此次数时，将该源站标记为不健康并暂时下线。</div>
                      </div>
                    </div>
                    <div class="sec-row">
                      <label class="sec-label">恢复上线阈值</label>
                      <div class="sec-value">
                        <div class="timeout-wrap">
                          <t-input-number
                            v-model="form.origin_health_check!.pass_threshold"
                            :min="1"
                            :max="20"
                            class="fld-timeout"
                          />
                          <span class="unit-suffix">次</span>
                        </div>
                        <div class="sec-hint">下线后连续探测成功达到此次数时，自动恢复上线。</div>
                      </div>
                    </div>
                  </template>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">回源协议</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">协议</label>
                    <div class="sec-value">
                      <t-radio-group v-model="form.origin_scheme">
                        <t-radio value="http">HTTP</t-radio>
                        <t-radio value="https">HTTPS</t-radio>
                        <t-radio value="follow_protocol">跟随客户端</t-radio>
                      </t-radio-group>
                      <div class="sec-hint">"跟随客户端"表示节点会根据请求来源协议自动选择回源 HTTP 或 HTTPS。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">回源端口</label>
                    <div class="sec-value">
                      <div class="port-wrap">
                        <t-input v-model="originPortText" type="number" class="fld-port" />
                        <t-button variant="outline" size="small" @click="form.origin_port = 80">80</t-button>
                        <t-button variant="outline" size="small" @click="form.origin_port = 443">443</t-button>
                        <t-button variant="outline" size="small" @click="form.origin_port = 8080">8080</t-button>
                      </div>
                      <div class="sec-hint">通常 HTTP 是 80，HTTPS 是 443。</div>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">回源域名</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">回源域名</label>
                    <div class="sec-value">
                      <t-radio-group v-model="form.origin_host_mode">
                        <t-radio value="request_host">访问域名</t-radio>
                        <t-radio value="request_host_port">访问域名:访问端口</t-radio>
                        <t-radio value="custom">自定义</t-radio>
                      </t-radio-group>
                      <div class="sec-hint">控制回源请求的 Host 头。多数源站匹配"访问域名"即可；当源站用不同端口区分虚拟主机时选"访问域名:访问端口"；需要固定 Host（例如对象存储 bucket 域名）时选"自定义"。</div>
                    </div>
                  </div>
                  <div class="sec-row" v-if="form.origin_host_mode === 'custom'">
                    <label class="sec-label">自定义 Host</label>
                    <div class="sec-value">
                      <t-input v-model="form.origin_host" placeholder="api.example.com" class="fld-default" />
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">回源超时</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">回源超时</label>
                    <div class="sec-value">
                      <div class="timeout-wrap">
                        <t-input-number
                          v-model="originTimeoutSec"
                          :min="1"
                          :max="600"
                          class="fld-timeout"
                        />
                        <span class="unit-suffix">秒</span>
                      </div>
                      <div class="sec-hint">整次请求响应的最长等待时间（包含源站处理 + 传输 body 的总耗时）。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">连接超时</label>
                    <div class="sec-value">
                      <div class="timeout-wrap">
                        <t-input-number
                          v-model="originConnectTimeoutSec"
                          :min="1"
                          :max="60"
                          class="fld-timeout"
                        />
                        <span class="unit-suffix">秒</span>
                      </div>
                      <div class="sec-hint">TCP 建连阶段的超时，对源站健康度最敏感；建议不超过 10 秒。</div>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">回源鉴权</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">开关</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.origin_auth!.enabled" />
                        <span class="switch-text">{{ form.origin_auth?.enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">开启后，节点会在回源时携带鉴权凭证，防止源站被直接访问（绕过 CDN）。</div>
                    </div>
                  </div>

                  <template v-if="form.origin_auth?.enabled">
                    <div class="sec-row">
                      <label class="sec-label">鉴权方式</label>
                      <div class="sec-value">
                        <t-radio-group v-model="form.origin_auth!.mode">
                          <t-radio value="header">自定义 Header</t-radio>
                          <t-radio value="basic">HTTP Basic Auth</t-radio>
                        </t-radio-group>
                        <div class="sec-hint">
                          自定义 Header：在回源请求中添加一个或多个自定义 HTTP 头（例如 <code>X-CDN-Auth: secret123</code>），源站校验该头即可拦截非 CDN 请求。<br>
                          HTTP Basic Auth：节点携带 <code>Authorization: Basic</code> 标准认证头，适用于源站已配置 Basic 认证的场景。
                        </div>
                      </div>
                    </div>

                    <!-- Mode: header -->
                    <template v-if="form.origin_auth!.mode === 'header'">
                      <div class="sec-row sec-row--top">
                        <label class="sec-label">认证 Header</label>
                        <div class="sec-value">
                          <div class="origin-auth-headers">
                            <div
                              v-for="(h, idx) in form.origin_auth!.headers"
                              :key="idx"
                              class="origin-auth-header-row"
                            >
                              <t-input
                                v-model="h.name"
                                placeholder="Header 名称 (如 X-CDN-Auth)"
                                class="fld-grow"
                              />
                              <t-input
                                v-model="h.value"
                                placeholder="Header 值 (如 secret123)"
                                class="fld-grow"
                                type="password"
                              />
                              <t-button
                                variant="text"
                                theme="danger"
                                :disabled="(form.origin_auth!.headers?.length ?? 0) <= 1"
                                @click="form.origin_auth!.headers!.splice(idx, 1)"
                              >删除</t-button>
                            </div>
                            <t-button variant="outline" size="small" @click="form.origin_auth!.headers!.push({ name: '', value: '' })">
                              <template #icon><t-icon name="add" /></template>
                              添加 Header
                            </t-button>
                          </div>
                          <div class="sec-hint">每行一组 Header，名称不区分大小写。值可包含任意字符串——建议使用随机密钥。</div>
                        </div>
                      </div>
                    </template>

                    <!-- Mode: basic -->
                    <template v-if="form.origin_auth!.mode === 'basic'">
                      <div class="sec-row">
                        <label class="sec-label">用户名</label>
                        <div class="sec-value">
                          <t-input v-model="form.origin_auth!.basic_user" placeholder="用户名" class="fld-creds" />
                        </div>
                      </div>
                      <div class="sec-row">
                        <label class="sec-label">密码</label>
                        <div class="sec-value">
                          <t-input v-model="form.origin_auth!.basic_pass" placeholder="密码" type="password" class="fld-creds" />
                        </div>
                      </div>
                    </template>
                  </template>
                </div>
              </section>

              <div class="form-actions">
                <t-button theme="primary" :loading="saving === 'origin'" :disabled="!originDirty" @click="saveOrigin">保存回源设置</t-button>
                <t-button variant="outline" :disabled="!originDirty || saving === 'origin'" @click="resetOrigin">重置</t-button>
                <span v-if="originDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </t-tab-panel>

          <!-- HTTPS设置 -->
          <t-tab-panel value="https">
            <template #label>
              <span class="tab-label"><ShieldCheck :size="14" /><span>HTTPS 设置</span></span>
            </template>
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">协议</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">HTTPS 开关</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.https_enabled" />
                        <span class="switch-text">{{ form.https_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">开启后节点会监听 443 端口并使用下方证书完成 TLS 握手。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">HTTP/2</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.http2_enabled" :disabled="!form.https_enabled" />
                        <span class="switch-text">{{ form.http2_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">HTTP/2 依赖 HTTPS，启用 HTTPS 后自动可切换。</div>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">证书</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">证书选择</label>
                    <div class="sec-value">
                      <t-select v-model="form.cert_id" :options="certOptions" class="fld-default" clearable />
                      <div class="sec-hint">未选择证书时，HTTPS 请求会返回默认证书或被拒绝。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">自动申请</label>
                    <div class="sec-value">
                      <div>
                        <t-button theme="primary" variant="outline" :loading="saving === 'acme'" @click="requestACME">
                          <template #icon><t-icon name="download" /></template>
                          一键申请 Let's Encrypt 证书
                        </t-button>
                      </div>
                      <div class="sec-hint">会为本域名申请 ACME 证书；需确认域名已正确 CNAME 到本 CDN。</div>
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <t-button theme="primary" :loading="saving === 'https'" :disabled="!httpsDirty" @click="saveHTTPS">保存 HTTPS 设置</t-button>
                <t-button variant="outline" :disabled="!httpsDirty || saving === 'https'" @click="resetHttps">重置</t-button>
                <span v-if="httpsDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </t-tab-panel>

          <!-- 缓存设置 -->
          <t-tab-panel value="cache">
            <template #label>
              <span class="tab-label"><HardDrive :size="14" /><span>缓存设置</span></span>
            </template>
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">缓存开关</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">全局缓存</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.cache_enabled" />
                        <span class="switch-text">{{ form.cache_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">关闭后本站所有响应都不会在边缘节点缓存。</div>
                    </div>
                  </div>
                </div>
                <div class="form-actions">
                  <t-button theme="primary" :loading="saving === 'cache'" :disabled="!cacheDirty" @click="saveCache">保存缓存开关</t-button>
                  <t-button variant="outline" :disabled="!cacheDirty || saving === 'cache'" @click="resetCache">重置</t-button>
                  <span v-if="cacheDirty" class="dirty-hint">有未保存的修改</span>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">命中本域名的缓存规则</h3>
                <t-alert theme="info" class="tab-alert">
                  <template #message>
                    {{ brand.title }} 的缓存规则按 <code>host_pattern</code> 匹配生效；
                    下表仅显示与「{{ domain?.name }}」匹配的规则（包含 <code>*</code> 通配）。
                    完整列表与跨域名规则请前往<t-link theme="primary" @click="goToCacheRulesPage">缓存规则页</t-link>。
                  </template>
                </t-alert>
                <t-table
                  :data="matchedCacheRules"
                  :columns="cacheRuleColumns"
                  row-key="id"
                  size="small"
                  bordered
                  empty="暂无命中规则"
                />
                <div class="form-actions">
                  <t-button theme="primary" variant="outline" @click="openCacheRuleDialog()">
                    <template #icon><t-icon name="add" /></template>
                    新增规则（预填本域名）
                  </t-button>
                </div>
              </section>
            </div>
          </t-tab-panel>

          <!-- 安全设置
               Layout follows the cdnfly-style screenshot:
                 · 默认防护 (single-choice radio card grid)
                 · 自动切换 (switch)
                 · 自定义规则 (table + bulk toolbar)
                 · 搜索引擎爬虫 (allow / deny / off)
                 · 更多设置 (collapse)
                 · 黑白名单 IP (two textareas)
               The backend today only persists the four Quick-CC fields
               (level/action/ban_seconds/fail_limit) globally; all new
               per-domain fields below are kept in local reactive state
               and a "保存" button calls saveCC which maps the chosen
               protection preset back onto the existing API. Extra fields
               (auto_switch / search_bot_action / custom rules / IP
               black/white lists) surface a 提示 so users know they are
               staged locally until the matching backend lands. -->
          <t-tab-panel value="security">
            <template #label>
              <span class="tab-label"><Shield :size="14" /><span>安全设置</span></span>
            </template>
            <div class="tab-body security-tab">
              <!-- Master switch: gates the whole block at compile time.
                   Lives outside the individual "CC 防护" section because
                   it controls blacklist + whitelist + custom rules too. -->
              <section class="config-section sec-master">
                <div class="sec-row">
                  <label class="sec-label">启用安全策略</label>
                  <div class="sec-value">
                    <div class="sec-switch">
                      <t-switch v-model="secForm.enabled" />
                      <span class="switch-text">{{ secForm.enabled ? '已启用' : '已关闭' }}</span>
                    </div>
                    <div class="sec-hint">
                      总开关。关闭后，以下 CC 防护、自定义规则、IP 黑白名单在节点层面整体失效（配置不会被清除，可随时重新开启）。
                    </div>
                  </div>
                </div>
              </section>

              <!-- Section 1: CC 防护 -->
              <section class="config-section">
                <h3 class="section-title">CC 防护</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">默认防护</label>
                    <div class="sec-value">
                      <t-radio-group v-model="secForm.default_mode" class="sec-mode-group">
                        <t-radio v-for="opt in ccDefaultModes" :key="opt.value" :value="opt.value">{{ opt.label }}</t-radio>
                      </t-radio-group>
                      <div class="sec-hint">当自动切换没有生效和下面的自定义规则没有匹配到时，就使用此处指定的默认防护</div>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">自动切换</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="secForm.auto_switch" />
                        <span class="switch-text">{{ secForm.auto_switch ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">开启后，节点会根据实时 QPS / 异常率自动在上面的防护等级间切换</div>
                    </div>
                  </div>

                  <div class="sec-row sec-row--top">
                    <label class="sec-label">自定义规则</label>
                    <div class="sec-value">
                      <div class="sec-toolbar">
                        <t-button theme="primary" @click="openCustomRule()">
                          <template #icon><t-icon name="add" /></template>
                          新增规则
                        </t-button>
                        <t-button variant="outline" @click="toggleAllCustomRules(true)" :disabled="secForm.custom_rules.length === 0">启用所有规则</t-button>
                        <t-button variant="outline" @click="toggleAllCustomRules(false)" :disabled="secForm.custom_rules.length === 0">关闭所有规则</t-button>
                      </div>
                      <t-table
                        :data="secForm.custom_rules"
                        :columns="customRuleColumns"
                        row-key="id"
                        size="small"
                        bordered
                        empty="暂无数据"
                        class="sec-rule-table"
                      />
                      <ol class="sec-hint sec-hint-list">
                        <li>自定义规则优先匹配，之后才是上面的默认防护；</li>
                        <li>像 API 请求的放行可以使用此处的自定义规则；</li>
                        <li>规则是从下到上匹配，可拖动规则调整顺序。</li>
                      </ol>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">搜索引擎爬虫</label>
                    <div class="sec-value">
                      <t-radio-group v-model="secForm.search_bot">
                        <t-radio value="off">不设置</t-radio>
                        <t-radio value="allow">放行</t-radio>
                        <t-radio value="deny">拦截</t-radio>
                      </t-radio-group>
                      <div class="sec-hint">爬虫包括谷歌、百度、搜狗、360 等</div>
                    </div>
                  </div>

                  <div class="sec-row sec-row--top">
                    <label class="sec-label">更多设置</label>
                    <div class="sec-value">
                      <t-collapse
                        v-model="secMoreExpanded"
                        :default-value="[]"
                        expand-icon-placement="right"
                        class="sec-more-collapse"
                      >
                        <t-collapse-panel header="封禁时长 / 挑战失败上限" :value="'more'">
                          <div class="sec-more">
                            <div class="sec-more-row">
                              <label>封禁时长 (秒)</label>
                              <t-input-number v-model="secForm.ban_seconds" :min="60" :max="86400" class="fld-mid" />
                            </div>
                            <div class="sec-more-row">
                              <label>挑战失败上限</label>
                              <t-input-number v-model="secForm.fail_limit" :min="1" :max="100" class="fld-mid" />
                            </div>
                          </div>
                        </t-collapse-panel>
                      </t-collapse>
                    </div>
                  </div>
                </div>
              </section>

              <!-- Section 2: 黑白名单 IP -->
              <section class="config-section">
                <h3 class="section-title">黑白名单 IP</h3>
                <div class="sec-form">
                  <div class="sec-row sec-row--top">
                    <label class="sec-label">黑名单</label>
                    <div class="sec-value">
                      <t-textarea
                        v-model="secForm.ip_blacklist"
                        placeholder="请输入 IP"
                        :autosize="{ minRows: 5, maxRows: 10 }"
                      />
                      <div class="sec-hint">一行一个，如 192.168.1.10、192.168.1.0/25，支持 # 作为注释</div>
                    </div>
                  </div>
                  <div class="sec-row sec-row--top">
                    <label class="sec-label">白名单</label>
                    <div class="sec-value">
                      <t-textarea
                        v-model="secForm.ip_whitelist"
                        placeholder="请输入 IP"
                        :autosize="{ minRows: 5, maxRows: 10 }"
                      />
                      <div class="sec-hint">一行一个，如 192.168.1.10、192.168.1.0/25，支持 # 作为注释</div>
                    </div>
                  </div>
                </div>
              </section>

              <!-- Section 3: 屏蔽设置 -->
              <section class="config-section">
                <h3 class="section-title">屏蔽设置</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">屏蔽透明代理</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="secForm.block_transparent_proxy" />
                        <span class="switch-text">{{ secForm.block_transparent_proxy ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">透明代理即网上免费公开的代理，带有 x-forwarded-for 请求头的</div>
                    </div>
                  </div>
                  <div class="sec-row sec-row--top">
                    <label class="sec-label">区域屏蔽</label>
                    <div class="sec-value">
                      <t-radio-group v-model="secForm.region_block_mode" class="region-mode-group">
                        <t-radio value="off">不设置</t-radio>
                        <t-radio value="foreign_exclude_hkmo_tw">国外（不包括港澳台）</t-radio>
                        <t-radio value="foreign_include_hkmo_tw">国外（包括港澳台）</t-radio>
                        <t-radio value="cn_include_hkmo_tw">中国（包括港澳台）</t-radio>
                        <t-radio value="cn_exclude_hkmo_tw">中国（不包括港澳台）</t-radio>
                        <t-radio value="custom">自定义</t-radio>
                      </t-radio-group>
                      <div v-if="secForm.region_block_mode === 'custom'" class="region-custom-wrap">
                        <t-textarea
                          v-model="secForm.custom_blocked_regions"
                          placeholder="请输入国家/地区代码，一行一个，例如：&#10;US&#10;JP&#10;KR&#10;RU"
                          :autosize="{ minRows: 4, maxRows: 10 }"
                        />
                        <div class="sec-hint">ISO 3166-1 国家/地区代码（两位大写字母），一行一个。常见代码：CN=中国 HK=香港 TW=台湾 MO=澳门 US=美国 JP=日本 KR=韩国 RU=俄罗斯 DE=德国 GB=英国 FR=法国 SG=新加坡 IN=印度</div>
                      </div>
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <t-button theme="primary" :loading="saving === 'cc'" @click="saveCC">保存安全设置</t-button>
              </div>
            </div>
          </t-tab-panel>


          <!-- 访问控制 -->
          <t-tab-panel value="access">
            <template #label>
              <span class="tab-label"><UserCheck :size="14" /><span>访问控制</span></span>
            </template>
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">全局 IP 白名单</h3>
                <t-alert theme="info" class="tab-alert">
                  <template #message>
                    目前 {{ brand.title }} 的 IP 白名单为 <b>全局作用域</b>——加入后所有域名一同生效。
                    单域名维度的黑/白名单与地域、UA 过滤将在后续版本提供。
                    管理请前往
                    <t-link theme="primary" @click="goToWAFPage">WAF 策略页</t-link>。
                  </template>
                </t-alert>
                <t-table
                  :data="whitelistEntries"
                  :columns="whitelistColumns"
                  row-key="id"
                  size="small"
                  bordered
                  empty="暂无白名单 IP"
                />
              </section>
            </div>
          </t-tab-panel>

          <!-- 高级设置 — listen_port moved to the basic tab. -->
          <t-tab-panel value="advanced">
            <template #label>
              <span class="tab-label"><Settings2 :size="14" /><span>高级设置</span></span>
            </template>
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">协议特性</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">WebSocket</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <t-switch v-model="form.websocket_enabled" />
                        <span class="switch-text">{{ form.websocket_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">允许 WS/WSS 连接升级（Upgrade: websocket）穿过 CDN 回源。</div>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">自定义错误页</h3>
                <div class="error-pages">
                  <div
                    v-for="(page, idx) in form.error_pages"
                    :key="idx"
                    class="error-page-row"
                  >
                    <t-input-number v-model="page.status" :min="100" :max="599" class="fld-status" />
                    <t-select v-model="page.mode" :options="errorPageModeOptions" class="fld-mode" />
                    <t-input v-model="page.content" placeholder="URL 或 HTML 内容" class="fld-flex" />
                    <t-button variant="outline" theme="danger" size="small" @click="removeErrorPage(idx)">删除</t-button>
                  </div>
                  <t-button variant="outline" size="small" @click="addErrorPage">+ 添加错误页规则</t-button>
                </div>
              </section>

              <div class="form-actions">
                <t-button theme="primary" :loading="saving === 'advanced'" :disabled="!advancedDirty" @click="saveAdvanced">保存高级设置</t-button>
                <t-button variant="outline" :disabled="!advancedDirty || saving === 'advanced'" @click="resetAdvanced">重置</t-button>
                <span v-if="advancedDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </t-tab-panel>
        </t-tabs>
      </t-card>
    </template>

    <t-card v-else class="section-card" bordered>
      <div class="empty-state">未找到该域名，可能已被删除。</div>
    </t-card>

    <!-- Cache rule editor dialog (from 缓存 tab) -->
    <t-dialog
      v-model:visible="cacheRuleDialogOpen"
      :header="cacheRuleEditing ? '编辑缓存规则' : '新建缓存规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitCacheRule"
    >
      <div class="form-grid">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <t-input v-model="cacheRuleForm.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">Host 模式</label>
          <t-input v-model="cacheRuleForm.host_pattern" placeholder="example.com 或 *.example.com 或 *" class="form-input-v" />
          <div class="form-hint-v">预填为当前域名。支持精确、单级通配（*.name）与全局（*）。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">路径模式</label>
          <t-input v-model="cacheRuleForm.path_pattern" placeholder="/* 或 /api/*" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">TTL (秒)</label>
          <t-input-number v-model="cacheRuleForm.ttl_seconds" :min="0" :max="31536000" class="form-input-v fld-cap-200" />
          <div class="form-hint-v">0 表示不缓存；建议静态资源 >= 3600。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">优先级</label>
          <t-input-number v-model="cacheRuleForm.priority" :min="1" :max="10000" class="form-input-v fld-cap-200" />
          <div class="form-hint-v">数值越大越先匹配。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">缓存 Query</label>
          <t-switch v-model="cacheRuleForm.cache_query_params" />
          <div class="form-hint-v">开启后 <code>?key=val</code> 会参与缓存键计算；否则按路径聚合命中。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <t-switch v-model="cacheRuleForm.enabled" />
        </div>
      </div>
    </t-dialog>

    <!-- Custom CC rule dialog (from 安全设置 tab) -->
    <t-dialog
      v-model:visible="customRuleDialog.open"
      :header="customRuleDialog.editingId ? '编辑自定义规则' : '新增自定义规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitCustomRule"
    >
      <div class="form-grid">
        <div class="form-row-v">
          <label class="form-label-v">匹配条件</label>
          <t-input
            v-model="customRuleDialog.form.match"
            placeholder="路径如 /api/*，或 header:X-Key=val，或 ua:bot"
            class="form-input-v"
          />
          <div class="form-hint-v">
            默认按路径前缀匹配；以 <code>header:</code> 或 <code>ua:</code> 开头启用对应模式。
          </div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">执行过滤</label>
          <t-radio-group v-model="customRuleDialog.form.filter">
            <t-radio value="放行">放行</t-radio>
            <t-radio value="拦截">拦截</t-radio>
            <t-radio value="验证">验证</t-radio>
          </t-radio-group>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">匹配模式</label>
          <t-select v-model="customRuleDialog.form.mode" class="fld-narrow-cap">
            <t-option v-for="opt in ccDefaultModes" :key="opt.value" :value="opt.value" :label="opt.label" />
          </t-select>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">备注</label>
          <t-input v-model="customRuleDialog.form.note" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <t-switch v-model="customRuleDialog.form.enabled" />
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import { DialogPlugin, MessagePlugin, Switch, Button } from "tdesign-vue-next"
import {
  ArrowLeft,
  Copy,
  RefreshCw,
  Globe2,
  Server,
  CloudUpload,
  ShieldCheck,
  HardDrive,
  Shield,
  UserCheck,
  Settings2,
  CheckCircle2,
  CircleSlash,
  Plug,
  Plus,
  Trash2,
  GripVertical,
  Sparkles,
} from "lucide-vue-next"
import {
  api,
  type CacheRule,
  type Certificate,
  type Cluster,
  type Domain,
  type DomainOrigin,
  type WAFWhitelistEntry,
} from "@/lib/api"
import { useAuthStore } from "@/stores/auth"
import { copyText } from "@/lib/clipboard"
import { useSystemSettings } from "@/lib/systemSettings"

const { brand } = useSystemSettings()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const loading = ref(true)
const saving = ref<string>("")

const domain = ref<Domain | null>(null)
const clusters = ref<Cluster[]>([])
const certs = ref<Certificate[]>([])

const activeTab = ref("basic")

// Editable form snapshot of the domain. We keep `domain.value` as the
// server-confirmed copy and only diff it back on save; per-tab saves patch
// their own fields to avoid racing with uncommitted edits on other tabs.
const form = reactive<Domain>({
  id: "",
  name: "",
  line_group_id: "",
  origin_id: "",
  origin_scheme: "http",
  origin_port: 80,
  origin_host_mode: "request_host",
  origin_host: "",
  origin_timeout_ms: 60000,
  origin_connect_timeout_ms: 10000,
  origin_auth: { enabled: false, mode: "header", headers: [], basic_user: "", basic_pass: "" },
  load_balance_method: "round_robin",
  origin_health_check: {
    enabled: false,
    interval_sec: 30,
    timeout_ms: 5000,
    path: "/",
    expected_status: 0,
    fail_threshold: 3,
    pass_threshold: 2,
  },
  cert_id: "",
  https_enabled: false,
  http2_enabled: false,
  listen_port: 0,
  error_pages: [],
  websocket_enabled: false,
  enabled: true,
  cache_enabled: true,
})

// --- Per-tab supporting state ---

// Cache rules: we fetch the global list once, filter by host_pattern
// relevance client-side, and let the user CRUD rules that happen to
// target this domain. Editing is gated to admin (the server already
// rejects non-admin writes for global resources).
const cacheRules = ref<CacheRule[]>([])
const cacheRuleDialogOpen = ref(false)
const cacheRuleEditing = ref<CacheRule | null>(null)
const cacheRuleForm = reactive<CacheRule>({
  id: "",
  name: "",
  host_pattern: "",
  path_pattern: "",
  methods: ["GET"],
  ttl_seconds: 300,
  cache_query_params: false,
  priority: 100,
  enabled: true,
})

// Global CC form mirrored from /api/waf/cc for the Quick CC panel.
const ccForm = reactive({
  level: "medium",
  action: "challenge",
  ban_seconds: 300,
  fail_limit: 3,
})

// -- Security tab (cdnfly-style "CC 防护 / 黑白名单 IP") ----------------
// The visual layout matches the reference screenshot but most fields
// here are staged locally until the backend gains per-domain
// protection settings. `saveCC` still writes `default_mode` (mapped
// via secModeToLegacyCC) plus `ban_seconds`/`fail_limit` to the
// existing global CC endpoint so the preset chosen here has a real
// server-side effect today.
interface SecCustomRule {
  id: string
  match: string      // e.g. "URI 包含 /api"
  filter: string     // "放行" / "拦截" / "验证码"
  mode: string       // radio preset key, same as default_mode
  note: string
  enabled: boolean
}

const ccDefaultModes = [
  { label: "关闭", value: "off" },
  { label: "宽松", value: "loose" },
  { label: "JS验证", value: "js" },
  { label: "5秒盾", value: "shield5s" },
  { label: "点击验证", value: "click" },
  { label: "滑块验证", value: "slide" },
  { label: "验证码", value: "captcha" },
  { label: "旋转图片", value: "rotate" },
  { label: "点击验证(简单)", value: "click_easy" },
  { label: "滑块验证(简单)", value: "slide_easy" },
  { label: "自定义", value: "custom" },
] as const

const secForm = reactive<{
  enabled: boolean
  default_mode: string
  auto_switch: boolean
  search_bot: "off" | "allow" | "deny"
  ban_seconds: number
  fail_limit: number
  custom_rules: SecCustomRule[]
  ip_blacklist: string
  ip_whitelist: string
  block_transparent_proxy: boolean
  region_block_mode: string
  custom_blocked_regions: string
}>({
  enabled: false,
  default_mode: "off",
  auto_switch: false,
  search_bot: "off",
  ban_seconds: 300,
  fail_limit: 3,
  custom_rules: [],
  ip_blacklist: "",
  ip_whitelist: "",
  block_transparent_proxy: false,
  region_block_mode: "off",
  custom_blocked_regions: "",
})
// "更多设置" 的展开状态由 <t-collapse v-model> 维护，默认收起。
// 绑数组以兼容 TCollapse 的多面板模式。
const secMoreExpanded = ref<string[]>([])

// Region blocking — resolve preset modes into blocked country code lists.
// The backend stores a flat `blocked_regions: string[]`, so the UI must
// expand presets before saving and detect them again on load.
const HKMO_TW = ["HK", "MO", "TW"]
const CN_CODES = ["CN"]

/** Expand the current region_block_mode + custom textarea into a flat
 *  array of ISO country codes for the backend. */
function resolveBlockedRegions(): string[] {
  switch (secForm.region_block_mode) {
    case "off":
      return []
    case "foreign_exclude_hkmo_tw":
      return ["__FOREIGN_EXCLUDE_HKMOTW__"]
    case "foreign_include_hkmo_tw":
      return ["__FOREIGN_INCLUDE_HKMOTW__"]
    case "cn_include_hkmo_tw":
      return [...CN_CODES, ...HKMO_TW]
    case "cn_exclude_hkmo_tw":
      return [...CN_CODES]
    case "custom":
      return secForm.custom_blocked_regions
        .split(/\r?\n/)
        .map((s) => s.trim().toUpperCase())
        .filter((s) => s.length === 2)
    default:
      return []
  }
}

/** Detect which mode a given blocked_regions list corresponds to,
 *  and hydrate secForm accordingly. */
function hydrateRegionBlock(regions: string[]) {
  if (!regions || regions.length === 0) {
    secForm.region_block_mode = "off"
    secForm.custom_blocked_regions = ""
    return
  }
  const set = new Set(regions.map((s) => s.toUpperCase()))
  if (set.has("__FOREIGN_EXCLUDE_HKMOTW__")) {
    secForm.region_block_mode = "foreign_exclude_hkmo_tw"
    secForm.custom_blocked_regions = ""
    return
  }
  if (set.has("__FOREIGN_INCLUDE_HKMOTW__")) {
    secForm.region_block_mode = "foreign_include_hkmo_tw"
    secForm.custom_blocked_regions = ""
    return
  }
  // CN only (no HK/MO/TW)
  if (set.size === 1 && set.has("CN")) {
    secForm.region_block_mode = "cn_exclude_hkmo_tw"
    secForm.custom_blocked_regions = ""
    return
  }
  // CN + HK + MO + TW
  if (set.size === 4 && set.has("CN") && set.has("HK") && set.has("MO") && set.has("TW")) {
    secForm.region_block_mode = "cn_include_hkmo_tw"
    secForm.custom_blocked_regions = ""
    return
  }
  // anything else → custom
  secForm.region_block_mode = "custom"
  secForm.custom_blocked_regions = regions.join("\n")
}

// Map the rich per-domain preset onto the 3-way legacy global CC API
// (low / medium / high) + challenge action, so saving the new UI still
// produces a sensible server-side effect until the backend grows real
// per-domain fields.
const secModeToLegacyCC = (mode: string): { level: string; action: string } => {
  switch (mode) {
    case "off":
      return { level: "low", action: "challenge" }
    case "loose":
    case "js":
      return { level: "low", action: "challenge" }
    case "click":
    case "click_easy":
    case "slide":
    case "slide_easy":
    case "captcha":
    case "rotate":
      return { level: "medium", action: "challenge" }
    case "shield5s":
      return { level: "high", action: "shield" }
    case "custom":
      return { level: "medium", action: "challenge" }
    default:
      return { level: "medium", action: "challenge" }
  }
}

const openCustomRule = (row?: SecCustomRule) => {
  // Two modes: create (no row) seeds a fresh rule; edit clones the
  // selected row into the dialog so Cancel discards changes cleanly.
  if (row) {
    customRuleDialog.editingId = row.id
    customRuleDialog.form = {
      id: row.id,
      match: row.match,
      filter: row.filter,
      mode: row.mode,
      note: row.note || "",
      enabled: row.enabled,
    }
  } else {
    customRuleDialog.editingId = ""
    customRuleDialog.form = {
      id: `rule-${Date.now()}`,
      match: "",
      filter: "放行",
      mode: "off",
      note: "",
      enabled: true,
    }
  }
  customRuleDialog.open = true
}

// Dialog state backing the 自定义规则 edit modal.
const customRuleDialog = reactive<{
  open: boolean
  editingId: string
  form: SecCustomRule
}>({
  open: false,
  editingId: "",
  form: {
    id: "",
    match: "",
    filter: "放行",
    mode: "off",
    note: "",
    enabled: true,
  },
})

const submitCustomRule = () => {
  const match = String(customRuleDialog.form.match || "").trim()
  if (!match) {
    MessagePlugin.error("请填写匹配条件")
    return
  }
  const row: SecCustomRule = { ...customRuleDialog.form, match }
  if (customRuleDialog.editingId) {
    const idx = secForm.custom_rules.findIndex((r) => r.id === customRuleDialog.editingId)
    if (idx >= 0) {
      secForm.custom_rules.splice(idx, 1, row)
    }
  } else {
    secForm.custom_rules.push(row)
  }
  customRuleDialog.open = false
}

const toggleAllCustomRules = (on: boolean) => {
  secForm.custom_rules.forEach((r) => (r.enabled = on))
}

const removeCustomRule = (id: string) => {
  secForm.custom_rules = secForm.custom_rules.filter((r) => r.id !== id)
}

const customRuleColumns = computed(() => [
  { colKey: "match", title: "匹配条件", minWidth: 180, ellipsis: true },
  { colKey: "filter", title: "执行过滤", width: 120 },
  {
    colKey: "mode",
    title: "匹配模式",
    width: 140,
    cell: (_h: any, { row }: { row: SecCustomRule }) => {
      const opt = ccDefaultModes.find((o) => o.value === row.mode)
      return opt ? opt.label : row.mode
    },
  },
  { colKey: "note", title: "备注", minWidth: 160, ellipsis: true },
  {
    colKey: "enabled",
    title: "状态",
    width: 90,
    cell: (_h: any, { row }: { row: SecCustomRule }) =>
      h(Switch, {
        size: "small",
        modelValue: Boolean(row.enabled),
        "onUpdate:modelValue": (v: boolean) => (row.enabled = Boolean(v)),
      }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    cell: (_h: any, { row }: { row: SecCustomRule }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(
          Button,
          {
            size: "small",
            theme: "primary",
            variant: "text",
            onClick: () => openCustomRule(row),
          },
          () => "编辑",
        ),
        h(
          Button,
          {
            size: "small",
            theme: "danger",
            variant: "text",
            onClick: () => removeCustomRule(row.id),
          },
          () => "删除",
        ),
      ]),
  },
])

// Whitelist entries — read-only in the detail view; IP add/remove lives
// on the standalone WAF page since the backend is global-scoped.
const whitelistEntries = ref<WAFWhitelistEntry[]>([])

// Per-domain origin rows. `domainOriginsForm` is the editable copy
// (reactive so v-model on inputs works); `domainOriginsBaseline` is a
// JSON-serialised snapshot of the server-side state, used by the dirty
// tracker and the "重置" button to revert without a round trip.
const domainOriginsForm = reactive<DomainOrigin[]>([])
const domainOriginsBaseline = ref<string>("[]")

const addDomainOrigin = () => {
  domainOriginsForm.push({ address: "", weight: 1, enabled: true })
}

const removeDomainOrigin = (idx: number) => {
  if (idx < 0 || idx >= domainOriginsForm.length) return
  domainOriginsForm.splice(idx, 1)
  if (domainOriginsForm.length === 0) {
    // Always leave at least one placeholder row — a domain with zero
    // enabled origins is a 502 waiting to happen. The save handler
    // still requires at least one non-empty address before PUT.
    domainOriginsForm.push({ address: "", weight: 1, enabled: true })
  }
}

const serializeOrigins = (list: DomainOrigin[]): string =>
  JSON.stringify(
    list.map(o => ({
      address: String(o.address || "").trim(),
      weight: Math.max(1, Math.min(100, Number(o.weight) || 1)),
      enabled: Boolean(o.enabled),
    })),
  )

const applyDomainOriginsSnapshot = (rows: DomainOrigin[]) => {
  domainOriginsForm.splice(0, domainOriginsForm.length)
  if (rows.length === 0) {
    domainOriginsForm.push({ address: "", weight: 1, enabled: true })
  } else {
    for (const r of rows) {
      domainOriginsForm.push({
        id: r.id,
        address: r.address || "",
        weight: Number(r.weight) > 0 ? Number(r.weight) : 1,
        enabled: r.enabled ?? true,
        sort_order: r.sort_order,
      })
    }
  }
  domainOriginsBaseline.value = serializeOrigins(domainOriginsForm)
}

const errorPageModeOptions = [
  { label: "内联 HTML", value: "inline" },
  { label: "重定向 URL", value: "redirect" },
  { label: "透传源站响应", value: "passthrough" },
]

const clusterOptions = computed(() =>
  (clusters.value || []).map(c => ({ label: c.name || c.id, value: c.id })),
)
const certOptions = computed(() =>
  (certs.value || []).map(c => ({ label: `${c.name || c.id} (${c.domain})`, value: c.id })),
)

const clusterName = computed(() => {
  if (!domain.value) return ""
  const id = domain.value.line_group_id || ""
  const hit = clusters.value.find(c => c.id === id)
  return hit?.name || domain.value.line_group_name || id || ""
})
const originSummary = computed(() => {
  if (!domain.value) return ""
  const addr = (domain.value.origin_addresses || []).join(", ")
  const name = domain.value.origin_name || ""
  if (name && addr) return `${name}（${addr}）`
  return name || addr || ""
})

const load = async () => {
  const id = String(route.params.id || "")
  if (!id) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    // Pull the main context in parallel. Secondary data (cache rules, WAF
    // policies, whitelist, cc) are loaded in a second wave so a primary-
    // fetch failure (e.g. domain not found) aborts early with a clean
    // empty state rather than half-populated tabs.
    const [domainsRes, clustersRes, certsRes] = await Promise.all([
      api.listDomains(),
      api.listClusters(),
      api.listCertificates(),
    ])
    const hit = (domainsRes.domains || []).find(d => d.id === id) || null
    domain.value = hit
    clusters.value = clustersRes.clusters || []
    certs.value = certsRes.certificates || []
    if (hit) {
      hydrateForm(hit)
      // Per-domain origins live under the domain, so we load them
      // together with the domain itself. Missing endpoints (older
      // backend) fall through to an empty list — the UI will render
      // the "please add an origin" empty row.
      try {
        const res = await api.listDomainOrigins(hit.id)
        applyDomainOriginsSnapshot(res.origins || [])
      } catch {
        applyDomainOriginsSnapshot([])
      }
      await loadSecondary(hit.id)
    }
  } catch (err: any) {
    MessagePlugin.error(err?.message || "加载失败")
  } finally {
    loading.value = false
  }
}

// Secondary-tab data. Each sub-fetch is independently try/catch'd so a
// backend quirk (e.g. empty policies table) doesn't paint the whole
// page broken. Failures are logged but surfaced as empty lists.
const loadSecondary = async (domainId: string) => {
  // The list endpoint (domainView) omits origin_auth. Fetch the full
  // domain so origin_auth is hydrated correctly — otherwise saving
  // origin settings silently resets auth to disabled defaults.
  try {
    const full = await api.getDomain(domainId)
    if (full) {
      // Merge: full domain fills in missing fields (origin_auth, security)
      // while preserving display-only fields from the list view
      // (listen_port, origin_name, cert_name, etc.).
      domain.value = { ...domain.value, ...full }
      // Re-hydrate only the fields the list endpoint was missing.
      const auth = full.origin_auth
      form.origin_auth = {
        enabled: auth?.enabled ?? false,
        mode: auth?.mode || "header",
        headers: (auth?.headers || []).map(h => ({ ...h })),
        basic_user: auth?.basic_user || "",
        basic_pass: auth?.basic_pass || "",
      }
    }
  } catch {
    // Non-fatal: the form keeps its list-endpoint values.
  }
  try {
    const res = await api.listCacheRules()
    cacheRules.value = res.cache_rules || []
  } catch {
    cacheRules.value = []
  }
  try {
    const res = await api.listWAFWhitelist()
    whitelistEntries.value = res.whitelist || []
  } catch {
    whitelistEntries.value = []
  }
  try {
    const cc = await api.getCCPolicy()
    ccForm.level = cc.level || "medium"
    ccForm.action = cc.action || "challenge"
    ccForm.ban_seconds = cc.ban_seconds || 300
    ccForm.fail_limit = cc.fail_limit || 3
  } catch {
    // keep defaults
  }
  try {
    // Pull the rich per-domain security blob and hydrate secForm so the
    // cdnfly-style 安全设置 tab reflects what's actually stored on the
    // domain (not just the legacy global CC policy).
    const sec = await api.getDomainSecurity(domainId)
    // Backend returns an explicit `enabled` bool for records that have it,
    // and keeps legacy rows (written before the master switch existed)
    // at enabled=true for back-compat.
    secForm.enabled = typeof sec.enabled === "boolean" ? sec.enabled : false
    secForm.default_mode = sec.default_mode || "off"
    secForm.auto_switch = Boolean(sec.auto_switch)
    secForm.search_bot = (sec.search_bot as any) || "off"
    secForm.ban_seconds = Number(sec.ban_seconds) || 300
    secForm.fail_limit = Number(sec.fail_limit) || 3
    secForm.custom_rules = (sec.custom_rules || []).map(r => ({
      id: r.id,
      match: r.match,
      filter: r.filter || "放行",
      mode: r.mode || "off",
      note: r.note || "",
      enabled: Boolean(r.enabled),
    }))
    secForm.ip_blacklist = (sec.ip_blacklist || []).join("\n")
    secForm.ip_whitelist = (sec.ip_whitelist || []).join("\n")
    secForm.block_transparent_proxy = Boolean(sec.block_transparent_proxy)
    hydrateRegionBlock(sec.blocked_regions || [])
  } catch {
    // first-save case: no security row yet — secForm keeps its reactive defaults.
  }
}

const hydrateForm = (d: Domain) => {
  form.id = d.id
  form.name = d.name
  form.line_group_id = d.line_group_id || ""
  form.origin_id = d.origin_id || ""
  form.origin_scheme = d.origin_scheme || "http"
  form.origin_port = d.origin_port ?? 80
  form.origin_host_mode = d.origin_host_mode || "request_host"
  form.origin_host = d.origin_host || ""
  form.origin_timeout_ms = d.origin_timeout_ms ?? 60000
  form.origin_connect_timeout_ms = d.origin_connect_timeout_ms ?? 10000
  // Hydrate origin_auth. Deep-copy so the form snapshot doesn't alias the server object.
  const auth = d.origin_auth
  form.origin_auth = {
    enabled: auth?.enabled ?? false,
    mode: auth?.mode || "header",
    headers: (auth?.headers || []).map(h => ({ ...h })),
    basic_user: auth?.basic_user || "",
    basic_pass: auth?.basic_pass || "",
  }
  // Load-balance method + active health check. Defaults match the
  // backend so newly-created domains land on weighted round_robin
  // with health-check disabled until the user opts in.
  form.load_balance_method = d.load_balance_method === "ip_hash" ? "ip_hash" : "round_robin"
  const hc = d.origin_health_check
  form.origin_health_check = {
    enabled: hc?.enabled ?? false,
    interval_sec: Number(hc?.interval_sec) > 0 ? Number(hc?.interval_sec) : 30,
    timeout_ms: Number(hc?.timeout_ms) > 0 ? Number(hc?.timeout_ms) : 5000,
    path: hc?.path || "/",
    expected_status: Number(hc?.expected_status) || 0,
    fail_threshold: Number(hc?.fail_threshold) > 0 ? Number(hc?.fail_threshold) : 3,
    pass_threshold: Number(hc?.pass_threshold) > 0 ? Number(hc?.pass_threshold) : 2,
  }
  form.cert_id = d.cert_id || ""
  form.https_enabled = Boolean(d.https_enabled)
  form.http2_enabled = Boolean(d.http2_enabled)
  form.listen_port = d.listen_port ?? 0
  form.error_pages = (d.error_pages || []).map(p => ({ ...p }))
  form.websocket_enabled = Boolean(d.websocket_enabled)
  form.enabled = d.enabled !== false
  form.cache_enabled = d.cache_enabled !== false
  form.cname = d.cname
}

const applyPatch = async (tab: string, patch: Partial<Domain>) => {
  if (!domain.value) return
  saving.value = tab
  try {
    const next: Domain = { ...domain.value, ...patch }
    const updated = await api.updateDomain(domain.value.id, next)
    domain.value = updated
    hydrateForm(updated)
    MessagePlugin.success("已保存")
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  } finally {
    saving.value = ""
  }
}

const saveBasic = () =>
  applyPatch("basic", {
    name: form.name,
    line_group_id: form.line_group_id,
    enabled: form.enabled,
    // HTTP listen port now lives in the basic tab (CDNfly-style layout).
    // Normalize empty/negative to 0 so the server uses the default.
    listen_port: Number(form.listen_port) || 0,
  })

// --- Basic tab dirty-state tracking ---
// Users kept pressing "保存" even when nothing changed, or worse, clicked
// away with unsaved edits. A derived dirty flag lets us disable 保存 when
// nothing's changed and surface a plain 重置 button that re-hydrates from
// the last-known-good server copy.
const basicDirty = computed(() => {
  const d = domain.value
  if (!d) return false
  return (
    form.name !== (d.name || "") ||
    form.line_group_id !== (d.line_group_id || "") ||
    Boolean(form.enabled) !== (d.enabled !== false) ||
    Number(form.listen_port || 0) !== Number(d.listen_port || 0)
  )
})
const resetBasic = () => {
  if (!domain.value) return
  const d = domain.value
  form.name = d.name
  form.line_group_id = d.line_group_id || ""
  form.enabled = d.enabled !== false
  form.listen_port = d.listen_port ?? 0
}

// --- Listen-port helpers ---
// The native <input type="number"> uses string models; we coerce through
// a computed to keep `form.listen_port` numeric and guard the 0-65535
// range. Preset buttons just write through setListenPort for one-click
// picks of the common 80 / 443 / 8080 values.
const listenPortText = computed<string>({
  get: () => (form.listen_port === 0 || form.listen_port == null ? "" : String(form.listen_port)),
  set: (v: string) => {
    const trimmed = String(v || "").trim()
    if (trimmed === "") { form.listen_port = 0; return }
    const n = Number(trimmed)
    if (!Number.isFinite(n)) { form.listen_port = 0; return }
    form.listen_port = Math.max(0, Math.min(65535, Math.round(n)))
  },
})
const setListenPort = (p: number) => {
  form.listen_port = Math.max(0, Math.min(65535, p))
}

// Origin port coerced through a string computed so the native numeric
// input stays clamped inside 1-65535 without relying on the stepper.
const originPortText = computed<string>({
  get: () => (form.origin_port == null ? "" : String(form.origin_port)),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    if (!Number.isFinite(n)) { form.origin_port = 80; return }
    form.origin_port = Math.max(1, Math.min(65535, Math.round(n)))
  },
})

// Auto-sync origin port with origin scheme to prevent the most common
// misconfiguration (http on :443 / https on :80) that causes origin nginx
// to return "400 Bad Request: The plain HTTP request was sent to HTTPS port".
// The node still has a runtime safeguard (reconcile_scheme_port), but nudging
// users toward the correct port at configure-time avoids the confusion
// entirely. We only auto-flip when the current port is the *other* scheme's
// default (80<->443) so users with custom ports (8080, 8443, etc.) are not
// disturbed. "follow_protocol" keeps whatever the user last chose.
watch(
  () => form.origin_scheme,
  (next, prev) => {
    // Skip the initial assignment during form hydration (prev === undefined)
    // so we never overwrite a freshly-loaded server value.
    if (prev === undefined) return
    if (next === prev) return
    const current = Number(form.origin_port || 0)
    if (next === "https" && current === 80) {
      form.origin_port = 443
    } else if (next === "http" && current === 443) {
      form.origin_port = 80
    }
    // "follow_protocol" / any other value: leave port alone.
  },
)

// Origin timeouts are stored as milliseconds on the backend for precision
// and to match existing API contracts, but the UI presents them in seconds
// because that's what operators actually reason about ("给源站 60 秒").
// Ceil the read side so a value like 59999ms still shows as 60; floor-clamp
// the write side so the user can't enter sub-second values that would be
// rounded to zero.
const originTimeoutSec = computed<number>({
  get: () => {
    const ms = Number(form.origin_timeout_ms ?? 0)
    if (!Number.isFinite(ms) || ms <= 0) return 60
    return Math.max(1, Math.round(ms / 1000))
  },
  set: (v: number) => {
    const n = Number(v)
    if (!Number.isFinite(n)) return
    form.origin_timeout_ms = Math.max(1, Math.min(600, Math.round(n))) * 1000
  },
})

const originConnectTimeoutSec = computed<number>({
  get: () => {
    const ms = Number(form.origin_connect_timeout_ms ?? 0)
    if (!Number.isFinite(ms) || ms <= 0) return 10
    return Math.max(1, Math.round(ms / 1000))
  },
  set: (v: number) => {
    const n = Number(v)
    if (!Number.isFinite(n)) return
    form.origin_connect_timeout_ms = Math.max(1, Math.min(60, Math.round(n))) * 1000
  },
})

// --- Per-tab dirty trackers ---
// Each tab's "保存" button is disabled unless the form diverges from the
// server-side copy; the 重置 button re-hydrates from the last-known-good
// server copy so users can discard an accidental edit without navigating
// away and losing work on other tabs.
const originDirty = computed(() => {
  const d = domain.value
  if (!d) return false
  // Server may store load_balance_method = "" for legacy rows; normalize
  // both sides to "round_robin" before comparing so an unchanged legacy
  // domain doesn't appear dirty just because the read-back differs.
  const serverLB = d.load_balance_method === "ip_hash" ? "ip_hash" : "round_robin"
  const formLB = form.load_balance_method === "ip_hash" ? "ip_hash" : "round_robin"
  // OriginHealthCheck: treat absent/zero struct on the server as the
  // same as the form's default (disabled with stock thresholds), so
  // toggling the switch off after a save reverts back to "clean".
  const defaultHC = {
    enabled: false,
    interval_sec: 30,
    timeout_ms: 5000,
    path: "/",
    expected_status: 0,
    fail_threshold: 3,
    pass_threshold: 2,
  }
  const serverHC = d.origin_health_check
    ? {
        enabled: Boolean(d.origin_health_check.enabled),
        interval_sec: Number(d.origin_health_check.interval_sec) > 0 ? Number(d.origin_health_check.interval_sec) : 30,
        timeout_ms: Number(d.origin_health_check.timeout_ms) > 0 ? Number(d.origin_health_check.timeout_ms) : 5000,
        path: d.origin_health_check.path || "/",
        expected_status: Number(d.origin_health_check.expected_status) || 0,
        fail_threshold: Number(d.origin_health_check.fail_threshold) > 0 ? Number(d.origin_health_check.fail_threshold) : 3,
        pass_threshold: Number(d.origin_health_check.pass_threshold) > 0 ? Number(d.origin_health_check.pass_threshold) : 2,
      }
    : defaultHC
  return (
    form.origin_scheme !== (d.origin_scheme || "http") ||
    Number(form.origin_port || 0) !== Number(d.origin_port || 0) ||
    form.origin_host_mode !== (d.origin_host_mode || "request_host") ||
    (form.origin_host || "") !== (d.origin_host || "") ||
    Number(form.origin_timeout_ms || 0) !== Number(d.origin_timeout_ms || 0) ||
    Number(form.origin_connect_timeout_ms || 0) !== Number(d.origin_connect_timeout_ms || 0) ||
    JSON.stringify(form.origin_auth) !== JSON.stringify(d.origin_auth || { enabled: false, mode: "header", headers: [], basic_user: "", basic_pass: "" }) ||
    formLB !== serverLB ||
    JSON.stringify(form.origin_health_check) !== JSON.stringify(serverHC) ||
    serializeOrigins(domainOriginsForm) !== domainOriginsBaseline.value
  )
})
const resetOrigin = () => {
  if (!domain.value) return
  hydrateForm(domain.value)
  // Re-apply the last-known-good origin list from the baseline.
  try {
    const snapshot: DomainOrigin[] = JSON.parse(domainOriginsBaseline.value)
    applyDomainOriginsSnapshot(snapshot)
  } catch {
    applyDomainOriginsSnapshot([])
  }
}

const httpsDirty = computed(() => {
  const d = domain.value
  if (!d) return false
  // saveHTTPS coerces http2 to false when https is off; mirror that
  // here so toggling https off after h2 was on still surfaces as
  // dirty (otherwise 保存 looks like a no-op while it actually flips
  // h2 server-side).
  const effectiveH2 = form.https_enabled ? Boolean(form.http2_enabled) : false
  return (
    Boolean(form.https_enabled) !== Boolean(d.https_enabled) ||
    effectiveH2 !== Boolean(d.http2_enabled) ||
    (form.cert_id || "") !== (d.cert_id || "")
  )
})
const resetHttps = () => {
  if (!domain.value) return
  hydrateForm(domain.value)
}

const cacheDirty = computed(() => {
  const d = domain.value
  if (!d) return false
  return Boolean(form.cache_enabled) !== (d.cache_enabled !== false)
})
const resetCache = () => {
  if (!domain.value) return
  hydrateForm(domain.value)
}

const advancedDirty = computed(() => {
  const d = domain.value
  if (!d) return false
  if (Boolean(form.websocket_enabled) !== Boolean(d.websocket_enabled)) return true
  const a = form.error_pages || []
  const b = d.error_pages || []
  if (a.length !== b.length) return true
  for (let i = 0; i < a.length; i++) {
    if (a[i].status !== b[i].status || a[i].mode !== b[i].mode || a[i].content !== b[i].content) return true
  }
  return false
})
const resetAdvanced = () => {
  if (!domain.value) return
  hydrateForm(domain.value)
}

const saveOrigin = async () => {
  if (!domain.value) return

  // Validate origin list first. At least one enabled entry with a
  // non-empty address is required; otherwise every request would hit
  // our "origin not found" 502 page.
  const entries = domainOriginsForm
    .map(o => ({
      address: String(o.address || "").trim(),
      weight: Math.max(1, Math.min(100, Number(o.weight) || 1)),
      enabled: Boolean(o.enabled),
    }))
    .filter(o => o.address !== "")
  if (entries.length === 0) {
    MessagePlugin.error("请至少填写一条回源地址")
    return
  }
  if (!entries.some(e => e.enabled)) {
    MessagePlugin.error("至少需要启用一条回源地址")
    return
  }

  saving.value = "origin"
  try {
    // Save domain-level origin settings and the origin list as two
    // sequential steps so a failure midway is easier to diagnose. Both
    // trigger auto-publish on the backend, but since the node reads
    // them together from the same config version, the order doesn't
    // affect correctness.
    const next: Domain = {
      ...domain.value,
      origin_scheme: form.origin_scheme,
      origin_port: form.origin_port,
      origin_host_mode: form.origin_host_mode,
      origin_host: form.origin_host,
      origin_timeout_ms: form.origin_timeout_ms,
      origin_connect_timeout_ms: form.origin_connect_timeout_ms,
      origin_auth: form.origin_auth,
      load_balance_method: form.load_balance_method,
      origin_health_check: form.origin_health_check,
    }
    const updated = await api.updateDomain(domain.value.id, next)
    domain.value = updated
    hydrateForm(updated)

    const originsRes = await api.replaceDomainOrigins(
      domain.value.id,
      entries as DomainOrigin[],
    )
    applyDomainOriginsSnapshot(originsRes.origins || [])
    MessagePlugin.success("已保存")
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  } finally {
    saving.value = ""
  }
}

const saveHTTPS = () =>
  applyPatch("https", {
    https_enabled: form.https_enabled,
    http2_enabled: form.https_enabled ? form.http2_enabled : false,
    cert_id: form.cert_id,
  })

const saveCache = () =>
  applyPatch("cache", {
    cache_enabled: form.cache_enabled,
  })

const saveAdvanced = () =>
  applyPatch("advanced", {
    error_pages: form.error_pages,
    websocket_enabled: form.websocket_enabled,
  })

// --- ACME certificate request ---
const requestACME = async () => {
  if (!domain.value) return
  saving.value = "acme"
  try {
    const res = await api.requestACMECertificate({ domain: domain.value.name })
    MessagePlugin.success(`证书申请已提交，到期时间 ${res.expires_at || "待定"}`)
    // Refresh cert list so the new cert can be selected immediately.
    const certRes = await api.listCertificates()
    certs.value = certRes.certificates || []
    // Re-hydrate the domain: the backend's persistACMECertificate auto-binds
    // the new cert_id AND flips https_enabled=true, so pulling the latest
    // domain record syncs the HTTPS tab without the user having to click
    // around the certificate dropdown and then save again.
    try {
      const refreshed = await api.getDomain(domain.value.id)
      if (refreshed) {
        domain.value = refreshed
        hydrateForm(refreshed)
      }
    } catch {
      // The cert was issued — a failed re-hydrate just means the form
      // won't auto-update, but a page refresh will show it correctly.
    }
  } catch (err: any) {
    MessagePlugin.error(err?.message || "申请证书失败；请确认域名已 CNAME 到本 CDN")
  } finally {
    saving.value = ""
  }
}

// --- Quick CC save ---
const saveCC = async () => {
  saving.value = "cc"
  try {
    // Derive the legacy global CC inputs from the rich secForm so the
    // tab's single "保存安全设置" button persists everything at once.
    const legacy = secModeToLegacyCC(secForm.default_mode)
    const id = String(route.params.id || "")
    if (!id) throw new Error("missing domain id")
    // Build the canonical DomainSecurity payload — split textareas, drop
    // blanks + # comments, and re-assign IDs server-side for new rules.
    const splitList = (s: string): string[] =>
      String(s || "")
        .split(/\r?\n/)
        .map(x => x.trim())
        .filter(x => x !== "" && !x.startsWith("#"))
    await api.updateDomainSecurity(id, {
      enabled: secForm.enabled,
      default_mode: secForm.default_mode,
      auto_switch: secForm.auto_switch,
      search_bot: secForm.search_bot,
      ban_seconds: secForm.ban_seconds,
      fail_limit: secForm.fail_limit,
      custom_rules: secForm.custom_rules.map(r => ({
        id: r.id,
        match: r.match,
        filter: r.filter,
        mode: r.mode,
        note: r.note,
        enabled: r.enabled,
      })),
      ip_blacklist: splitList(secForm.ip_blacklist),
      ip_whitelist: splitList(secForm.ip_whitelist),
      block_transparent_proxy: secForm.block_transparent_proxy,
      blocked_regions: resolveBlockedRegions(),
    })
    // Also update the legacy global CC policy so the global CC dashboard
    // reflects the domain's protection level in the same click — but
    // ONLY for admins: non-admin users get 403 from that endpoint (which
    // is intentional — it writes global state) and the failure would
    // mask the successful per-domain save above. Admins still need the
    // side-effect so the global dashboard stays in sync.
    if (isAdmin.value) {
      try {
        await api.setCCPolicy({
          level: legacy.level,
          action: legacy.action,
          ban_seconds: secForm.ban_seconds,
          fail_limit: secForm.fail_limit,
        })
      } catch (err: any) {
        // The per-domain save already landed; warn but don't roll back.
        MessagePlugin.warning(
          "域名安全设置已保存，但同步全局 CC 策略失败：" + (err?.message || "未知错误")
        )
        return
      }
    }
    MessagePlugin.success("安全设置已保存")
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  } finally {
    saving.value = ""
  }
}

// --- Per-domain cache rule filter ---
// Match a rule to the current domain if its host_pattern either equals
// the domain name, is the literal '*' wildcard, or ends with '.<domain>'
// (subdomain-wildcard). Anything else is considered 'unrelated'.
const matchedCacheRules = computed<CacheRule[]>(() => {
  const name = domain.value?.name || ""
  if (!name) return []
  return cacheRules.value.filter(rule => {
    const p = (rule.host_pattern || "").trim()
    if (!p) return false
    if (p === "*" || p === name) return true
    if (p.startsWith("*.") && name.endsWith(p.slice(1))) return true
    if (p.startsWith(".") && name.endsWith(p)) return true
    return false
  })
})

const cacheRuleColumns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 160 },
  { colKey: "host_pattern", title: "Host 模式", width: 180 },
  { colKey: "path_pattern", title: "路径模式", minWidth: 160 },
  {
    colKey: "ttl_seconds",
    title: "TTL (秒)",
    width: 110,
    cell: (_h: any, { row }: { row: CacheRule }) => String(row.ttl_seconds || 0),
  },
  {
    colKey: "enabled",
    title: "启用",
    width: 90,
    cell: (_h: any, { row }: { row: CacheRule }) =>
      h(Switch, {
        size: "small",
        modelValue: row.enabled,
        "onUpdate:modelValue": (v: boolean) => toggleCacheRule(row, v),
      }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 130,
    cell: (_h: any, { row }: { row: CacheRule }) =>
      h("div", { class: "row-actions" }, [
        h(Button, { size: "small", theme: "primary", variant: "text", onClick: () => openCacheRuleDialog(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => confirmDeleteCacheRule(row) }, () => "删除"),
      ]),
  },
])

const openCacheRuleDialog = (rule?: CacheRule) => {
  cacheRuleEditing.value = rule || null
  if (rule) {
    Object.assign(cacheRuleForm, rule)
  } else {
    // Create path: prefill host_pattern with the current domain so the
    // new rule is immediately relevant to this detail view.
    cacheRuleForm.id = ""
    cacheRuleForm.name = `${domain.value?.name || ""} 规则`
    cacheRuleForm.host_pattern = domain.value?.name || ""
    cacheRuleForm.path_pattern = "/*"
    cacheRuleForm.methods = ["GET"]
    cacheRuleForm.ttl_seconds = 300
    cacheRuleForm.cache_query_params = false
    cacheRuleForm.priority = 100
    cacheRuleForm.enabled = true
  }
  cacheRuleDialogOpen.value = true
}

const submitCacheRule = async () => {
  try {
    if (cacheRuleEditing.value) {
      await api.updateCacheRule(cacheRuleEditing.value.id, cacheRuleForm)
      MessagePlugin.success("规则已更新")
    } else {
      await api.createCacheRule(cacheRuleForm)
      MessagePlugin.success("规则已创建")
    }
    cacheRuleDialogOpen.value = false
    const res = await api.listCacheRules()
    cacheRules.value = res.cache_rules || []
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  }
}

const toggleCacheRule = async (rule: CacheRule, enabled: boolean) => {
  try {
    await api.updateCacheRule(rule.id, { ...rule, enabled })
    const res = await api.listCacheRules()
    cacheRules.value = res.cache_rules || []
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新失败")
  }
}

const confirmDeleteCacheRule = (rule: CacheRule) => {
  const inst = DialogPlugin.confirm({
    header: "删除缓存规则",
    body: `确认删除规则「${rule.name || rule.id}」吗？此操作不可撤销。`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    onConfirm: async () => {
      try {
        await api.deleteCacheRule(rule.id)
        MessagePlugin.success("规则已删除")
        const res = await api.listCacheRules()
        cacheRules.value = res.cache_rules || []
      } catch (err: any) {
        MessagePlugin.error(err?.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

// --- Whitelist table (read-only) ---
const whitelistColumns = computed(() => [
  { colKey: "ip", title: "IP / CIDR", minWidth: 160 },
  { colKey: "note", title: "备注", minWidth: 200 },
  { colKey: "created_by", title: "创建者", width: 140 },
  {
    colKey: "created_at",
    title: "创建时间",
    width: 180,
    cell: (_h: any, { row }: { row: WAFWhitelistEntry }) => formatTime(row.created_at),
  },
])

const addErrorPage = () => {
  const list = form.error_pages || []
  list.push({ status: 404, mode: "inline", content: "" })
  form.error_pages = list
}

const removeErrorPage = (idx: number) => {
  const list = form.error_pages || []
  list.splice(idx, 1)
  form.error_pages = list
}

const copyCname = async () => {
  if (!domain.value?.cname) return
  const ok = await copyText(domain.value.cname)
  if (ok) {
    MessagePlugin.success("CNAME 已复制")
  } else {
    MessagePlugin.warning("复制失败，请手动选择复制")
  }
}

const regenCname = async () => {
  if (!domain.value) return
  // CNAME regeneration is destructive — user's DNS records pointing at
  // the old CNAME go dark until they re-aim at the new one. Require
  // explicit confirmation so a misclick can't take a domain offline.
  const inst = DialogPlugin.confirm({
    header: "重新生成 CNAME",
    body: `重新生成后，旧 CNAME「${domain.value.cname || '-'}」会立即失效，所有已配置的 DNS 记录需要同步更新，否则本域名将无法访问。是否继续？`,
    theme: "warning",
    confirmBtn: { content: "重新生成", theme: "warning" },
    cancelBtn: "取消",
    onConfirm: async () => {
      if (!domain.value) { inst.destroy(); return }
      saving.value = "regen-cname"
      try {
        const next: Domain = { ...domain.value, cname: "" }
        const updated = await api.updateDomain(domain.value.id, next)
        domain.value = updated
        hydrateForm(updated)
        MessagePlugin.success(`CNAME 已重新生成: ${updated.cname}`)
      } catch (err: any) {
        MessagePlugin.error(err?.message || "重新生成 CNAME 失败")
      } finally {
        saving.value = ""
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const isAdmin = computed(() => auth.user?.role === "admin")

// Hero 健康摘要：把 4 个最关键的开关（HTTPS / 缓存 / 安全策略 / WS）
// 聚合成一组只读 chip，让用户进入页面就能扫到当前接入网站的整体健康度，
// 不必再点开每个 tab 才知道。
const healthChips = computed(() => [
  { key: "https", label: "HTTPS", on: !!form.https_enabled, icon: ShieldCheck },
  { key: "cache", label: "缓存", on: !!form.cache_enabled, icon: HardDrive },
  { key: "security", label: "安全策略", on: !!secForm.enabled, icon: Shield },
  { key: "ws", label: "WebSocket", on: !!form.websocket_enabled, icon: Plug },
])

const goBack = () => {
  router.push(isAdmin.value ? "/admin/dashboard/domains" : "/dashboard/domains")
}

const goToCacheRulesPage = () => {
  router.push(isAdmin.value ? "/admin/dashboard/cache-rules" : "/dashboard/rules")
}

const goToWAFPage = () => {
  // WAF console has a dedicated admin route; non-admins share the user
  // rules view (they can't edit global WAF anyway).
  router.push(isAdmin.value ? "/admin/dashboard/waf" : "/dashboard/rules")
}

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

// Full timestamp including the user's local timezone offset, shown in
// the tooltip so operators working across regions can disambiguate a
// 2026/4/18 20:34 stamp without guessing UTC vs local.
const formatTimeFull = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  const local = d.toLocaleString("zh-CN", { hour12: false })
  // Offset in minutes; negate to match the conventional "UTC+08:00" sign.
  const offsetMin = -d.getTimezoneOffset()
  const sign = offsetMin >= 0 ? "+" : "-"
  const abs = Math.abs(offsetMin)
  const hh = String(Math.floor(abs / 60)).padStart(2, "0")
  const mm = String(abs % 60).padStart(2, "0")
  return `${local} (UTC${sign}${hh}:${mm})`
}

onMounted(load)
</script>

<style scoped>
.page {
  padding: 24px;
  max-width: 1280px;
  margin: 0 auto;
}

/* Width utility classes for form fields. Centralizes the width
 * literals previously inlined as `style="width:XXXpx"` across the 7
 * config tabs, so spacing stays coherent and easy to retune. */
.fld-port { width: 140px; }
.fld-status { width: 120px; }
.fld-mid { width: 200px; }
.fld-mode { width: 160px; }
.fld-timeout { width: 240px; }
.fld-creds { max-width: 320px; }
.fld-default { max-width: 420px; }
.fld-narrow-cap { max-width: 260px; }
.fld-cap-200 { max-width: 200px; }
.fld-grow { flex: 1; min-width: 160px; }
.fld-flex { flex: 1; }

.loading-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 320px;
}

/* ─── Hero ──────────────────────────────────────────────────────────────
 * Gradient brand banner that anchors the page. The two aria-hidden
 * children (.hero-aurora + .hero-grid) layer a subtle radial glow and a
 * fine grid pattern over the gradient so the header feels alive without
 * needing background images. All colors come from tokens.css.
 */
.hero {
  position: relative;
  overflow: hidden;
  border-radius: var(--app-card-radius, 12px);
  background: linear-gradient(135deg, #2D2899 0%, #4D44E0 45%, #8E5BFF 100%);
  color: #ffffff;
  padding: 24px 28px 22px;
  margin-bottom: var(--app-page-gap, 20px);
  box-shadow: var(--app-brand-shadow-md, 0 4px 14px rgba(99, 91, 255, 0.22));
  isolation: isolate;
}
.hero-aurora {
  position: absolute;
  inset: -40% -10% auto -10%;
  height: 220%;
  background:
    radial-gradient(50% 60% at 80% 20%, rgba(125, 211, 252, 0.45), transparent 60%),
    radial-gradient(40% 50% at 15% 90%, rgba(99, 91, 255, 0.55), transparent 60%);
  filter: blur(20px);
  pointer-events: none;
  z-index: 0;
}
.hero-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(to right, rgba(255, 255, 255, 0.06) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(255, 255, 255, 0.06) 1px, transparent 1px);
  background-size: 32px 32px;
  mask-image: radial-gradient(ellipse at top right, #000 30%, transparent 75%);
  -webkit-mask-image: radial-gradient(ellipse at top right, #000 30%, transparent 75%);
  pointer-events: none;
  z-index: 0;
}
.hero-content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.hero-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.hero-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.92);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, transform 0.15s;
}
.hero-back:hover {
  background: rgba(255, 255, 255, 0.2);
  transform: translateX(-2px);
}
.hero-status :deep(.t-tag) {
  font-weight: 500;
}
.hero-main {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.hero-icon {
  flex: 0 0 auto;
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.22);
  color: #e0f2fe;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18);
}
.hero-title-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  flex: 1;
}
.hero-eyebrow {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.66);
  letter-spacing: 0.5px;
  text-transform: uppercase;
}
.hero-title {
  font-size: 24px;
  font-weight: 600;
  line-height: 1.2;
  margin: 0;
  word-break: break-all;
}
.hero-cname {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 2px;
}
.hero-cname code {
  background: rgba(15, 23, 42, 0.32);
  border: 1px solid rgba(255, 255, 255, 0.16);
  color: #e0f2fe;
  padding: 3px 10px;
  border-radius: 6px;
  font-size: 13px;
  font-family: "JetBrains Mono", "Fira Code", Consolas, monospace;
  word-break: break-all;
}
.hero-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.86);
  cursor: pointer;
  transition: background 0.15s, transform 0.15s;
}
.hero-icon-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.22);
  transform: translateY(-1px);
}
.hero-icon-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Health chips: at-a-glance status row beneath the title. Each chip is
 * a brand-tinted pill that changes background based on its on/off state
 * so users can scan the row in <1s without parsing labels. */
.hero-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}
.hchip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: 999px;
  font-size: 12px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(6px);
}
.hchip-on {
  background: rgba(16, 185, 129, 0.22);
  border-color: rgba(110, 231, 183, 0.45);
  color: #d1fae5;
}
.hchip-off {
  background: rgba(15, 23, 42, 0.28);
  border-color: rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.62);
}
.hchip-state {
  font-weight: 500;
  opacity: 0.92;
}

/* ─── Info card (TDescriptions) ───────────────────────────────────── */
.info-card {
  margin-bottom: var(--app-page-gap, 20px);
  border-radius: var(--app-card-radius, 12px) !important;
  box-shadow: var(--app-shadow-card, 0 1px 3px rgba(15, 23, 42, 0.04));
}
.info-card-header {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 15px;
  color: var(--app-text-strong);
}
.info-card-icon {
  color: var(--app-brand);
}
.info-desc :deep(.t-descriptions__header),
.info-desc :deep(.t-descriptions__label) {
  font-weight: 500;
}
.cn-code {
  background: var(--td-bg-color-component, #f6f7fa);
  padding: 2px 8px;
  border-radius: 4px;
  font-family: "JetBrains Mono", "Fira Code", Consolas, monospace;
  font-size: 13px;
  color: var(--app-text-strong);
  word-break: break-all;
}

/* ─── Tab labels with icon ───────────────────────────────────────── */
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}
.tab-label svg {
  color: currentColor;
  opacity: 0.85;
}

.section-card {
  margin-bottom: 16px;
  border-radius: var(--app-card-radius, 12px) !important;
}

.info-grid,
.info-row,
.info-label,
.info-value,
.cname-value,
.cname-value code,
.status-dot,
.status-dot.status-ok {
  /* Legacy info-grid superseded by <t-descriptions>; kept as no-op selectors
   * so any rogue references in older snapshots compile cleanly. */
}

/* Grouped section + stacked form style to match the CDNfly basic-config
 * layout screenshot: a section title, then each field stacked vertically
 * with its own hint directly underneath instead of inline. Reserved the
 * existing .form-row / .form-label / .form-hint rules for the older tabs
 * (origin / https / advanced) which don't need the new layout yet. */
.config-section {
  padding: 8px 4px 4px;
  border-bottom: 1px solid var(--td-border-level-1-color, #e7e7e7);
  margin-bottom: 16px;
}
.config-section:last-of-type {
  border-bottom: none;
  margin-bottom: 0;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 16px;
  padding: 2px 0 2px 12px;
  position: relative;
  color: var(--app-text-strong, #0f172a);
  letter-spacing: 0.2px;
}
.section-title::before {
  content: "";
  position: absolute;
  left: 0;
  top: 4px;
  bottom: 4px;
  width: 4px;
  border-radius: 4px;
  background: linear-gradient(180deg, #9385FE 0%, #4D44E0 100%);
  box-shadow: 0 1px 4px rgba(99, 91, 255, 0.4);
}
.form-grid {
  /* Two-column form on desktop. Each child is a .form-row-v cell.
   * Users reading the previous single-column layout on wide screens
   * complained that the page "looked like a phone" — all fields were
   * stacked vertically and the right half of the card stayed blank.
   * Switching to auto-fit gives the desktop a proper two-column form
   * while the @media block below collapses back to one column on
   * narrow viewports. */
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px 32px;
}
.form-row-v {
  display: grid;
  grid-template-columns: 110px minmax(0, 1fr);
  gap: 6px 16px;
  align-items: center;
  min-width: 0;
}
.form-label-v {
  color: var(--td-text-color-secondary, #94a3b8);
  text-align: right;
  font-size: 14px;
}
.form-input-v {
  width: 100%;
  min-width: 0;
  max-width: 420px;
}
.form-hint-v {
  grid-column: 2;
  color: var(--td-text-color-placeholder, #a9a9a9);
  font-size: 12px;
  margin-top: 2px;
}

.tab-body {
  padding: 16px 4px 4px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.tab-alert {
  margin-bottom: 8px;
}

.form-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.form-label {
  width: 120px;
  flex-shrink: 0;
  padding-top: 6px;
  color: var(--td-text-color-secondary, #666);
}

.form-hint {
  flex-basis: 100%;
  margin-left: 132px;
  color: var(--td-text-color-placeholder, #999);
  font-size: 12px;
}

.form-actions {
  display: flex;
  gap: 12px;
  padding: 12px 16px;
  border-top: 1px solid var(--app-divider, #f1f5f9);
  margin-top: 16px;
  align-items: center;
  position: sticky;
  bottom: 0;
  /* Frosted backdrop so the sticky bar stays readable when content
   * scrolls underneath it (long tabs like 安全设置 / 回源设置). */
  background: linear-gradient(to top, var(--app-surface, #fff) 70%, rgba(255, 255, 255, 0.85));
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border-radius: 0 0 var(--app-card-radius, 12px) var(--app-card-radius, 12px);
  z-index: 5;
}

/* Switch row: compact switch + caption, so it stops visually reading as
 * a progress bar when placed inside a stretchy grid cell. */
.switch-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}
.switch-caption {
  color: var(--td-text-color-secondary, #94a3b8);
  font-size: 13px;
}

/* Port row: numeric input + preset buttons inline. Presets save users
 * from stepper-clicking 80 → 443 → 8080. */
.port-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

/* Timeout inputs show the unit ("秒") inline to the right of the input so
 * the operator can scan 60 + 秒 at a glance without reading the hint text. */
.timeout-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}
.unit-suffix {
  color: var(--td-text-color-secondary, #666);
  font-size: 14px;
  user-select: none;
}

/* Inline note next to the disabled origin-auth switch so the UI makes it
 * obvious this is an upcoming feature rather than a broken control. */
.switch-hint {
  margin-left: 10px;
  color: var(--td-text-color-placeholder, #999);
  font-size: 12px;
  user-select: none;
}

/* Dirty hint next to the 保存 button so the user sees unsaved state
 * without needing a toast. */
.dirty-hint {
  color: var(--td-warning-color, #e37318);
  font-size: 12px;
}

.error-pages {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
}

.error-page-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.empty-state {
  padding: 48px;
  text-align: center;
  color: var(--app-text-faint);
}

@media (max-width: 768px) {
  .page {
    padding: 12px;
  }

  /* Hero collapses to a single column with smaller title and tighter
   * padding; chips wrap to two rows on phones without overflowing. */
  .hero {
    padding: 18px 18px 16px;
  }
  .hero-title {
    font-size: 19px;
  }
  .hero-icon {
    width: 44px;
    height: 44px;
  }
  .hero-main {
    gap: 12px;
  }
  .hero-cname {
    width: 100%;
  }

  /* Origin list collapses to a vertical card on phones — the 5-column
   * grid is too tight at <420px. We drop the head row and stack each
   * field with its label tag inline. */
  .origin-list-head {
    display: none;
  }
  .origin-row {
    grid-template-columns: 1fr;
    gap: 8px;
    padding: 10px;
    border: 1px solid var(--app-divider, #f1f5f9);
  }
  .origin-handle {
    display: none;
  }

  .form-row-v {
    grid-template-columns: 1fr;
    gap: 4px;
  }

  .form-label-v {
    text-align: left;
  }

  .form-hint-v {
    grid-column: 1;
  }

  .form-row {
    flex-direction: column;
    align-items: stretch;
  }

  .form-label {
    width: auto;
    padding-top: 0;
  }

  .form-hint {
    margin-left: 0;
  }

  .form-actions {
    flex-wrap: wrap;
  }

  .error-page-row {
    flex-wrap: wrap;
  }
}

/* ─── Security-style horizontal form layout ────────────────────────────
 * Used in the security tab and now unified across all config tabs.
 * Each .sec-row is a horizontal label‒value pair:
 *   .sec-label (fixed left)  |  .sec-value (fluid right)
 * This mirrors the CDNfly / Aliyun / Tencent Cloud detail-page style:
 * labels on the left, controls on the right, hints beneath the control.
 */
.sec-form {
  display: flex;
  flex-direction: column;
  gap: 0;
}
.sec-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 14px 8px;
  border-bottom: 1px solid var(--td-border-level-1-color, #f0f0f0);
}
.sec-row:last-child {
  border-bottom: none;
}
.sec-row--top {
  align-items: flex-start;
}
.sec-label {
  width: 120px;
  min-width: 120px;
  flex-shrink: 0;
  color: var(--td-text-color-secondary, #94a3b8);
  font-size: 14px;
  text-align: right;
  padding-top: 2px;
}
.sec-value {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sec-hint {
  color: var(--td-text-color-placeholder, #a9a9a9);
  font-size: 12px;
  line-height: 1.6;
}
.sec-hint-list {
  padding-left: 18px;
  margin: 4px 0 0;
}

/* Region blocking radio group + custom textarea */
.region-mode-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.region-custom-wrap {
  margin-top: 8px;
}

.sec-mode-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.sec-toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.sec-rule-table {
  margin-bottom: 4px;
}
.sec-more {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 8px;
  padding: 12px;
  background: var(--td-bg-color-component, #f6f7fa);
  border-radius: 6px;
}
.sec-more-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.sec-more-row label {
  min-width: 100px;
  color: var(--td-text-color-secondary, #94a3b8);
  font-size: 13px;
  text-align: right;
}
.sec-master {
  background: var(--td-bg-color-container-hover, #f9f9fa);
  border-radius: 6px;
  padding: 12px 8px;
  margin-bottom: 8px;
}

/* Origin auth header editor */
.origin-auth-headers {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.origin-auth-header-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

/* ─── Origin list (回源地址) ──────────────────────────────────────────
 * Bug fix: the template referenced these classes but the previous CSS
 * file never defined them, so the rows collapsed to default block
 * layout — input, weight, switch and 删除 stacked vertically with no
 * spacing. We now lay them out as a 5-column grid mirroring the head
 * row, with a left drag handle (visual cue only — drag to reorder is
 * a future enhancement) and a polished inline 删除 icon button.
 */
.origin-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--app-border, #e2e8f0);
  border-radius: 10px;
  background: var(--app-surface, #ffffff);
  padding: 6px;
}
.origin-list-head,
.origin-row {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) 110px 130px 36px;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 8px;
}
.origin-list-head {
  padding: 6px 8px 4px;
  border-bottom: 1px dashed var(--app-divider, #f1f5f9);
}
.origin-col {
  font-size: 12px;
  color: var(--app-text-faint, #94a3b8);
  text-transform: uppercase;
  letter-spacing: 0.4px;
}
.origin-col-addr { grid-column: 2; }
.origin-col-weight { grid-column: 3; }
.origin-col-enabled { grid-column: 4; }
.origin-col-action { grid-column: 5; }
.origin-row {
  background: var(--app-surface, #ffffff);
  transition: background 0.15s var(--app-easing-standard, cubic-bezier(0.4, 0, 0.2, 1));
}
.origin-row:hover {
  background: var(--app-surface-hover, #f1f5f9);
}
.origin-handle {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--app-text-faint, #94a3b8);
  cursor: grab;
  user-select: none;
}
.origin-address {
  width: 100%;
}
.origin-weight {
  display: flex;
  align-items: center;
}
.origin-enabled {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--app-text-muted, #64748b);
  font-size: 12px;
}
.origin-del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--app-text-muted, #64748b);
  cursor: pointer;
  transition: background 0.15s, color 0.15s, border-color 0.15s;
}
.origin-del:hover:not(:disabled) {
  background: var(--app-danger-soft-bg, #fef2f2);
  color: var(--app-danger, #ef4444);
  border-color: var(--app-danger-soft-bg, #fef2f2);
}
.origin-del:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.origin-add {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  align-self: flex-start;
  margin-top: 4px;
  padding: 7px 14px;
  border-radius: 8px;
  background: var(--app-brand-soft-bg, rgba(99, 91, 255, 0.08));
  color: var(--app-brand-strong, #4D44E0);
  border: 1px dashed var(--app-brand-soft-border, rgba(99, 91, 255, 0.35));
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.origin-add:hover {
  background: rgba(99, 91, 255, 0.14);
  border-style: solid;
}

/* Cache rule action cells: bug fix — the cell renderer used `.row-actions`
 * (1855) without a matching style, so 编辑/删除 buttons sat with no gap. */
.row-actions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

/* TDesign collapse used inside the security tab (更多设置). Dim the
 * default arrow color and tighten padding so it visually sits inside our
 * sec-row label/value grid. */
.sec-more-collapse :deep(.t-collapse-panel) {
  border-radius: 8px;
  background: var(--app-surface-muted, #f8fafc);
  border: 1px solid var(--app-border, #e2e8f0);
  padding: 0 12px;
}
.sec-more-collapse :deep(.t-collapse-panel__header) {
  padding: 10px 0;
  font-size: 14px;
}
.sec-more-collapse :deep(.t-collapse-panel__body) {
  padding-bottom: 12px;
}

/* Switch control: consistent wrap + caption for all toggles */
.sec-switch {
  display: flex;
  align-items: center;
  gap: 10px;
}
.sec-switch .switch-text {
  color: var(--td-text-color-secondary, #94a3b8);
  font-size: 13px;
  user-select: none;
}

@media (max-width: 768px) {
  .sec-row {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
    padding: 10px 4px;
  }
  .sec-label {
    width: auto;
    min-width: auto;
    text-align: left;
  }
  .sec-more-row {
    flex-direction: column;
    align-items: stretch;
  }
  .sec-more-row label {
    text-align: left;
  }
}

/*
 * Domain detail tabs — 7 tabs with Chinese labels need ~700px of horizontal
 * space. On narrow/mid viewports TDesign's default can drop the active tab
 * off-screen because of how its resize observer measures a sibling wrapper.
 * Force horizontal scrolling explicitly.
 */
@media (max-width: 1080px) {
  .domain-detail-tabs .t-tabs__nav-scroll,
  .domain-detail-tabs .t-tabs__nav {
    overflow-x: auto;
    flex-wrap: nowrap;
    -webkit-overflow-scrolling: touch;
  }
  .domain-detail-tabs .t-tabs__nav-item {
    flex: 0 0 auto;
  }
}
</style>
