<template>
  <div class="page">
    <SkeletonPage v-if="loading" />
    <ErrorState v-else-if="error" :message="error" @retry="load" />

    <template v-else-if="domain">
      <div class="domain-page-head">
        <el-button link type="primary" class="domain-page-back" aria-label="返回域名列表" @click="goBack">
          <ArrowLeft :size="18" />
        </el-button>
        <h1 class="domain-page-title">{{ domain.name }}</h1>
      </div>

      <PermissionNotice
        v-if="!isAdmin && hasRestrictions && (!canEditDomain || !canEditSecurity)"
        :message="readonlyNotice"
      />
      <el-card class="section-card domain-detail-tabs-wrap" shadow="never">
        <el-tabs v-model="activeTab" class="admin-tabs domain-detail-tabs">
          <el-tab-pane label="基本配置" name="basic">
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">基本信息</h3>
                <div class="sec-form sec-form--info">
                  <div class="sec-row">
                    <label class="sec-label">状态</label>
                    <div class="sec-value">
                      <span class="domain-status-line">
                        <span class="status-dot" :class="domain.enabled ? 'status-ok' : 'status-off'" />
                        {{ domain.enabled ? "正常" : "已停用" }}
                      </span>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">CNAME</label>
                    <div class="sec-value">
                      <div class="cname-inline">
                        <code class="cn-code">{{ domain.cname || "尚未生成" }}</code>
                        <el-button v-if="domain.cname" link type="primary" title="复制 CNAME" @click="copyCname">
                          <Copy :size="14" />
                        </el-button>
                        <el-button
                          v-if="canEditDomain"
                          link
                          type="primary"
                          :loading="saving === 'regen-cname'"
                          @click="regenCname"
                        >
                          更改 CNAME
                        </el-button>
                      </div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">套餐</label>
                    <div class="sec-value sec-value--text">{{ clusterName || "-" }}</div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">源站</label>
                    <div class="sec-value sec-value--text">{{ originSummary || "-" }}</div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">创建时间</label>
                    <div class="sec-value sec-value--text">
                      <span :title="formatTimeFull(domain.created_at)">{{ formatTime(domain.created_at) }}</span>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">更新时间</label>
                    <div class="sec-value sec-value--text">
                      <span :title="formatTimeFull(domain.updated_at)">{{ formatTime(domain.updated_at) }}</span>
                    </div>
                  </div>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">基本设置</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">套餐</label>
                    <div class="sec-value">
                      <EpSelect v-model="form.line_group_id" :options="clusterOptions" placeholder="请选择" class="fld-default" />
                      <div class="sec-hint">切换套餐并不会导致 CNAME 地址变动，只是会指向新的节点服务器。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">域名</label>
                    <div class="sec-value">
                      <el-input v-model="form.name" class="fld-default" />
                      <div class="sec-hint">多个域名请用空格分隔；修改后 CNAME 可能重新生成。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">网站启停</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.enabled" />
                        <span class="switch-text">{{ form.enabled ? "已启用" : "已停用" }}</span>
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
                    <label class="sec-label">开关</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="httpAccessEnabled" />
                        <span class="switch-text">{{ httpAccessEnabled ? "已启用" : "已关闭" }}</span>
                      </div>
                      <div class="sec-hint">如果关闭，网站将无法使用 HTTP 访问；建议同时开启 HTTPS。</div>
                    </div>
                  </div>
                  <div v-show="httpAccessEnabled" class="sec-row">
                    <label class="sec-label">监听端口</label>
                    <div class="sec-value">
                      <div class="port-wrap">
                        <el-input v-model="listenPortText" type="number" class="fld-port" placeholder="80" />
                        <el-button plain size="small" @click="setListenPort(80)">80</el-button>
                        <el-button plain size="small" @click="setListenPort(443)">443</el-button>
                        <el-button plain size="small" @click="setListenPort(8080)">8080</el-button>
                        <el-button link size="small" @click="setListenPort(0)">默认</el-button>
                      </div>
                      <div class="sec-hint">留空或 0 使用默认端口 80。</div>
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <el-button v-if="canEditDomain" type="primary" :loading="saving === 'basic'" :disabled="!basicDirty" @click="saveBasic">保存</el-button>
                <el-button plain :disabled="!basicDirty || saving === 'basic'" @click="resetBasic">重置</el-button>
                <span v-if="basicDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </el-tab-pane>

          <!-- 回源设置 -->
          <el-tab-pane label="回源设置" name="origin">
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
                          <el-input
                            v-model="entry.address"
                            placeholder="1.2.3.4 或 origin.example.com:8080"
                            class="origin-address"
                          />
                          <div class="origin-weight">
                            <el-input-number
                              v-model="entry.weight"
                              :min="1"
                              :max="100"
                              class="fld-status"
                            />
                          </div>
                          <div class="origin-enabled">
                            <el-switch v-model="entry.enabled" size="small" />
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
                      <el-radio-group v-model="form.load_balance_method">
                        <el-radio value="round_robin">轮循</el-radio>
                        <el-radio value="ip_hash">定源</el-radio>
                      </el-radio-group>
                      <div class="sec-hint">当添加多个源站时，负载方式为轮循时，请求平均地转发到各个源站，当为 IP Hash 时，同一个用户的请求固定发往一个源站，一般用于会话保持。</div>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">源站健康检查</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.origin_health_check!.enabled" />
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
                          <el-input-number
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
                        <el-input v-model="form.origin_health_check!.path" placeholder="/" class="fld-default" />
                        <div class="sec-hint">节点会向 <code>http://源站地址{{ form.origin_health_check?.path || '/' }}</code> 发送 GET 请求。建议使用源站上的轻量级健康检查端点（例如 <code>/healthz</code>）。</div>
                      </div>
                    </div>
                    <div class="sec-row">
                      <label class="sec-label">超时时间</label>
                      <div class="sec-value">
                        <div class="timeout-wrap">
                          <el-input-number
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
                          <el-input-number
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
                          <el-input-number
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
                      <el-radio-group v-model="form.origin_scheme">
                        <el-radio value="http">HTTP</el-radio>
                        <el-radio value="https">HTTPS</el-radio>
                        <el-radio value="follow_protocol">跟随客户端</el-radio>
                      </el-radio-group>
                      <div class="sec-hint">"跟随客户端"表示节点会根据请求来源协议自动选择回源 HTTP 或 HTTPS。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">回源端口</label>
                    <div class="sec-value">
                      <div class="port-wrap">
                        <el-input v-model="originPortText" type="number" class="fld-port" />
                        <el-button plain size="small" @click="form.origin_port = 80">80</el-button>
                        <el-button plain size="small" @click="form.origin_port = 443">443</el-button>
                        <el-button plain size="small" @click="form.origin_port = 8080">8080</el-button>
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
                      <el-radio-group v-model="form.origin_host_mode">
                        <el-radio value="request_host">访问域名</el-radio>
                        <el-radio value="request_host_port">访问域名:访问端口</el-radio>
                        <el-radio value="custom">自定义</el-radio>
                      </el-radio-group>
                      <div class="sec-hint">控制回源请求的 Host 头。多数源站匹配"访问域名"即可；当源站用不同端口区分虚拟主机时选"访问域名:访问端口"；需要固定 Host（例如对象存储 bucket 域名）时选"自定义"。</div>
                    </div>
                  </div>
                  <div class="sec-row" v-if="form.origin_host_mode === 'custom'">
                    <label class="sec-label">自定义 Host</label>
                    <div class="sec-value">
                      <el-input v-model="form.origin_host" placeholder="api.example.com" class="fld-default" />
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
                        <el-input-number
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
                        <el-input-number
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
                        <el-switch v-model="form.origin_auth!.enabled" />
                        <span class="switch-text">{{ form.origin_auth?.enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">开启后，节点会在回源时携带鉴权凭证，防止源站被直接访问（绕过 CDN）。</div>
                    </div>
                  </div>

                  <template v-if="form.origin_auth?.enabled">
                    <div class="sec-row">
                      <label class="sec-label">鉴权方式</label>
                      <div class="sec-value">
                        <el-radio-group v-model="form.origin_auth!.mode">
                          <el-radio value="header">自定义 Header</el-radio>
                          <el-radio value="basic">HTTP Basic Auth</el-radio>
                        </el-radio-group>
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
                              <el-input
                                v-model="h.name"
                                placeholder="Header 名称 (如 X-CDN-Auth)"
                                class="fld-grow"
                              />
                              <el-input
                                v-model="h.value"
                                placeholder="Header 值 (如 secret123)"
                                class="fld-grow"
                                type="password"
                              />
                              <el-button
                                link
                                type="danger"
                                :disabled="(form.origin_auth!.headers?.length ?? 0) <= 1"
                                @click="form.origin_auth!.headers!.splice(idx, 1)"
                              >删除</el-button>
                            </div>
                            <el-button plain size="small" @click="form.origin_auth!.headers!.push({ name: '', value: '' })">
                              <template #icon><EpIcon name="add" /></template>
                              添加 Header
                            </el-button>
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
                          <el-input v-model="form.origin_auth!.basic_user" placeholder="用户名" class="fld-creds" />
                        </div>
                      </div>
                      <div class="sec-row">
                        <label class="sec-label">密码</label>
                        <div class="sec-value">
                          <el-input v-model="form.origin_auth!.basic_pass" placeholder="密码" type="password" class="fld-creds" />
                        </div>
                      </div>
                    </template>
                  </template>
                </div>
              </section>

              <div class="form-actions">
                <el-button v-if="canEditDomain" type="primary" :loading="saving === 'origin'" :disabled="!originDirty" @click="saveOrigin">保存回源设置</el-button>
                <el-button plain :disabled="!originDirty || saving === 'origin'" @click="resetOrigin">重置</el-button>
                <span v-if="originDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </el-tab-pane>

          <el-tab-pane label="HTTPS 设置" name="https">
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">协议</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">HTTPS 开关</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.https_enabled" />
                        <span class="switch-text">{{ form.https_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">开启后节点会监听 443 端口并使用下方证书完成 TLS 握手。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">HTTP/2</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.http2_enabled" :disabled="!form.https_enabled" />
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
                      <EpSelect v-model="form.cert_id" :options="certOptions" class="fld-default" clearable />
                      <div class="sec-hint">未选择证书时，HTTPS 请求会返回默认证书或被拒绝。</div>
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">自动申请</label>
                    <div class="sec-value">
                      <div>
                        <el-button type="primary" plain :loading="saving === 'acme'" @click="requestACME">
                          <template #icon><EpIcon name="download" /></template>
                          一键申请 Let's Encrypt 证书
                        </el-button>
                      </div>
                      <div class="sec-hint">会为本域名申请 ACME 证书；需确认域名已正确 CNAME 到本 CDN。</div>
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <el-button v-if="canEditDomain" type="primary" :loading="saving === 'https'" :disabled="!httpsDirty" @click="saveHTTPS">保存 HTTPS 设置</el-button>
                <el-button plain :disabled="!httpsDirty || saving === 'https'" @click="resetHttps">重置</el-button>
                <span v-if="httpsDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </el-tab-pane>

          <el-tab-pane label="安全设置" name="security">
            <div class="tab-body security-tab">
              <!-- No master switch: each sub-field below is its own
                   independent toggle, so an empty form means no edge
                   policy. The compiler's behaviour matches that exactly. -->

              <!-- Section 1: CC 防护 -->
              <section class="config-section">
                <h3 class="section-title">CC 防护</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">默认防护</label>
                    <div class="sec-value">
                      <el-radio-group v-model="secForm.default_mode" class="sec-mode-group">
                        <el-radio v-for="opt in ccDefaultModes" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
                      </el-radio-group>
                      <div class="sec-hint">当自动切换没有生效和下面的自定义规则没有匹配到时，就使用此处指定的默认防护</div>
                    </div>
                  </div>

                  <div class="sec-row">
                    <label class="sec-label">自动切换</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="secForm.auto_switch" />
                        <span class="switch-text">{{ secForm.auto_switch ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">自动切换功能开发中，当前仅保存默认防护模式；请手动选择并保存。</div>
                    </div>
                  </div>

                  <div class="sec-row sec-row--top">
                    <label class="sec-label">自定义规则</label>
                    <div class="sec-value">
                      <div class="sec-toolbar">
                        <el-button type="primary" @click="openCustomRule()">
                          <template #icon><EpIcon name="add" /></template>
                          新增规则
                        </el-button>
                        <el-button plain @click="toggleAllCustomRules(true)" :disabled="secForm.custom_rules.length === 0">启用所有规则</el-button>
                        <el-button plain @click="toggleAllCustomRules(false)" :disabled="secForm.custom_rules.length === 0">关闭所有规则</el-button>
                      </div>
                      <EpDataTable
                        :data="secForm.custom_rules"
                        :columns="customRuleColumns"
                        row-key="id"
                        size="small"
                        empty-text="暂无数据"
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
                      <el-radio-group v-model="secForm.search_bot">
                        <el-radio value="off">不设置</el-radio>
                        <el-radio value="allow">放行</el-radio>
                        <el-radio value="deny">拦截</el-radio>
                      </el-radio-group>
                      <div class="sec-hint">爬虫包括谷歌、百度、搜狗、360 等</div>
                    </div>
                  </div>

                  <div class="sec-row sec-row--top">
                    <label class="sec-label">更多设置</label>
                    <div class="sec-value">
                      <el-collapse
                        v-model="secMoreExpanded"
                        :default-value="[]"
                        expand-icon-placement="right"
                        class="sec-more-collapse"
                      >
                        <el-collapse-item header="封禁时长 / 挑战失败上限" :name="'more'">
                          <div class="sec-more">
                            <div class="sec-more-row">
                              <label>封禁时长 (秒)</label>
                              <el-input-number v-model="secForm.ban_seconds" :min="60" :max="86400" class="fld-mid" />
                            </div>
                            <div class="sec-more-row">
                              <label>挑战失败上限</label>
                              <el-input-number v-model="secForm.fail_limit" :min="1" :max="100" class="fld-mid" />
                            </div>
                          </div>
                        </el-collapse-item>
                      </el-collapse>
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
                      <el-input
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
                      <el-input
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
                        <el-switch v-model="secForm.block_transparent_proxy" />
                        <span class="switch-text">{{ secForm.block_transparent_proxy ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">透明代理即网上免费公开的代理，带有 x-forwarded-for 请求头的</div>
                    </div>
                  </div>
                  <div class="sec-row sec-row--top">
                    <label class="sec-label">区域屏蔽</label>
                    <div class="sec-value">
                      <el-radio-group v-model="secForm.region_block_mode" class="region-mode-group">
                        <el-radio value="off">不设置</el-radio>
                        <el-radio value="foreign_exclude_hkmo_tw">国外（不包括港澳台）</el-radio>
                        <el-radio value="foreign_include_hkmo_tw">国外（包括港澳台）</el-radio>
                        <el-radio value="cn_include_hkmo_tw">中国（包括港澳台）</el-radio>
                        <el-radio value="cn_exclude_hkmo_tw">中国（不包括港澳台）</el-radio>
                        <el-radio value="custom">自定义</el-radio>
                      </el-radio-group>
                      <div v-if="secForm.region_block_mode === 'custom'" class="region-custom-wrap">
                        <el-input
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

              <section class="config-section">
                <h3 class="section-title">边缘增强</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">Bot 评分</label>
                    <div class="sec-value"><el-switch v-model="secForm.bot_score_enabled" /></div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">响应压缩</label>
                    <div class="sec-value"><el-switch v-model="secForm.response_compress_enabled" /></div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">图片变换</label>
                    <div class="sec-value"><el-switch v-model="secForm.image_transform_enabled" /></div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">视频分片缓存</label>
                    <div class="sec-value"><el-switch v-model="secForm.video_segment_cache_enabled" /></div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">Edge Script</label>
                    <div class="sec-value"><el-switch v-model="secForm.edge_script_enabled" /></div>
                  </div>
                  <div v-if="secForm.edge_script_enabled" class="sec-row align-top">
                    <label class="sec-label">脚本规则 (JSON)</label>
                    <div class="sec-value">
                      <el-input v-model="secForm.edge_script_rules" :autosize="{ minRows: 3 }" placeholder='[{"match":"path:/api/*","action":"block"}]' />
                    </div>
                  </div>
                  <div class="sec-row">
                    <label class="sec-label">签名 URL 密钥</label>
                    <div class="sec-value">
                      <el-input v-model="secForm.signed_url_secret" type="password" placeholder="留空则关闭签名 URL 访问" class="fld-default" />
                    </div>
                  </div>
                </div>
              </section>

              <div class="form-actions">
                <el-button v-if="canEditSecurity" type="primary" :loading="saving === 'cc'" @click="saveCC">保存安全设置</el-button>
              </div>
            </div>
          </el-tab-pane>


          <el-tab-pane label="缓存设置" name="cache">
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">缓存开关</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">全局缓存</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.cache_enabled" />
                        <span class="switch-text">{{ form.cache_enabled ? '已启用' : '已关闭' }}</span>
                      </div>
                      <div class="sec-hint">关闭后本站所有响应都不会在边缘节点缓存。</div>
                    </div>
                  </div>
                </div>
                <div class="form-actions">
                  <el-button v-if="canEditDomain" type="primary" :loading="saving === 'cache'" :disabled="!cacheDirty" @click="saveCache">保存缓存开关</el-button>
                  <el-button plain :disabled="!cacheDirty || saving === 'cache'" @click="resetCache">重置</el-button>
                  <span v-if="cacheDirty" class="dirty-hint">有未保存的修改</span>
                </div>
              </section>

              <section class="config-section">
                <h3 class="section-title">命中本域名的缓存规则</h3>
                <el-alert theme="info" class="tab-alert">
                  <template #message>
                    {{ brand.title }} 的缓存规则按 <code>host_pattern</code> 匹配生效；
                    下表仅显示与「{{ domain?.name }}」匹配的规则（包含 <code>*</code> 通配）。
                    完整列表与跨域名规则请前往<el-link type="primary" @click="goToCacheRulesPage">缓存规则页</el-link>。
                  </template>
                </el-alert>
                <EpDataTable
                  :data="matchedCacheRules"
                  :columns="cacheRuleColumns"
                  row-key="id"
                  size="small"
                  empty-text="暂无命中规则"
                />
                <div class="form-actions">
                  <el-button type="primary" plain @click="openCacheRuleDialog()">
                    <template #icon><EpIcon name="add" /></template>
                    新增规则（预填本域名）
                  </el-button>
                </div>
              </section>
            </div>
          </el-tab-pane>

          <el-tab-pane label="访问控制" name="access">
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">全局 IP 白名单</h3>
                <el-alert theme="info" class="tab-alert">
                  <template #message>
                    全局 WAF IP 白名单（所有域名生效）请在
                    <el-link type="primary" @click="goToWAFPage">WAF 策略页</el-link>
                    管理。单域名的 IP 黑/白名单、地域封锁与 CC 规则请在「安全防护」Tab 配置并保存。
                  </template>
                </el-alert>
                <EpDataTable
                  :data="whitelistEntries"
                  :columns="whitelistColumns"
                  row-key="id"
                  size="small"
                  empty-text="暂无白名单 IP"
                />
              </section>
            </div>
          </el-tab-pane>

          <el-tab-pane label="高级设置" name="advanced">
            <div class="tab-body">
              <section class="config-section">
                <h3 class="section-title">协议特性</h3>
                <div class="sec-form">
                  <div class="sec-row">
                    <label class="sec-label">WebSocket</label>
                    <div class="sec-value">
                      <div class="sec-switch">
                        <el-switch v-model="form.websocket_enabled" />
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
                    <el-input-number v-model="page.status" :min="100" :max="599" class="fld-status" />
                    <EpSelect v-model="page.mode" :options="errorPageModeOptions" class="fld-mode" />
                    <el-input v-model="page.content" placeholder="URL 或 HTML 内容" class="fld-flex" />
                    <el-button plain type="danger" size="small" @click="removeErrorPage(idx)">删除</el-button>
                  </div>
                  <el-button plain size="small" @click="addErrorPage">+ 添加错误页规则</el-button>
                </div>
              </section>

              <div class="form-actions">
                <el-button v-if="canEditDomain" type="primary" :loading="saving === 'advanced'" :disabled="!advancedDirty" @click="saveAdvanced">保存高级设置</el-button>
                <el-button plain :disabled="!advancedDirty || saving === 'advanced'" @click="resetAdvanced">重置</el-button>
                <span v-if="advancedDirty" class="dirty-hint">有未保存的修改</span>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-card>
    </template>

    <el-card v-else class="section-card">
      <div class="empty-state">未找到该域名，可能已被删除。</div>
    </el-card>

    <!-- Cache rule editor dialog (from 缓存 tab) -->
    <EpDialog append-to-body
      v-model="cacheRuleDialogOpen"
      :title="cacheRuleEditing ? '编辑缓存规则' : '新建缓存规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitCacheRule"
    >
      <div class="domain-form-grid">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <el-input v-model="cacheRuleForm.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">Host 模式</label>
          <el-input v-model="cacheRuleForm.host_pattern" placeholder="example.com 或 *.example.com 或 *" class="form-input-v" />
          <div class="form-hint-v">预填为当前域名。支持精确、单级通配（*.name）与全局（*）。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">路径模式</label>
          <el-input v-model="cacheRuleForm.path_pattern" placeholder="/* 或 /api/*" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">TTL (秒)</label>
          <el-input-number v-model="cacheRuleForm.ttl_seconds" :min="0" :max="31536000" class="form-input-v fld-cap-200" />
          <div class="form-hint-v">0 表示不缓存；建议静态资源 >= 3600。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">优先级</label>
          <el-input-number v-model="cacheRuleForm.priority" :min="1" :max="10000" class="form-input-v fld-cap-200" />
          <div class="form-hint-v">数值越大越先匹配。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">缓存 Query</label>
          <el-switch v-model="cacheRuleForm.cache_query_params" />
          <div class="form-hint-v">开启后 <code>?key=val</code> 会参与缓存键计算；否则按路径聚合命中。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="cacheRuleForm.enabled" />
        </div>
      </div>
    </EpDialog>

    <!-- Custom CC rule dialog (from 安全设置 tab) -->
    <EpDialog append-to-body
      v-model="customRuleDialog.open"
      :title="customRuleDialog.editingId ? '编辑自定义规则' : '新增自定义规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitCustomRule"
    >
      <div class="domain-form-grid">
        <div class="form-row-v">
          <label class="form-label-v">匹配条件</label>
          <el-input
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
          <el-radio-group v-model="customRuleDialog.form.filter">
            <el-radio value="放行">放行</el-radio>
            <el-radio value="拦截">拦截</el-radio>
            <el-radio value="验证">验证</el-radio>
          </el-radio-group>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">匹配模式</label>
          <el-select v-model="customRuleDialog.form.mode" class="fld-narrow-cap">
            <el-option v-for="opt in ccDefaultModes" :key="opt.value" :value="opt.value" :label="opt.label" />
          </el-select>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">备注</label>
          <el-input v-model="customRuleDialog.form.note" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="customRuleDialog.form.enabled" />
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, reactive, ref, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElSwitch } from "element-plus"
import {
  ArrowLeft,
  Copy,
  Plus,
  Trash2,
  GripVertical,
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
import PermissionNotice from "@/components/common/PermissionNotice.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"
import { copyText } from "@/lib/clipboard"
import { useSystemSettings } from "@/lib/systemSettings"
import ErrorState from "@/components/common/ErrorState.vue"
import SkeletonPage from "@/components/common/SkeletonPage.vue"

const { brand } = useSystemSettings()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { canDomainsWrite, canWAFEdit, hasRestrictions } = useUserPermissions()

const loading = ref(true)
const error = ref("")
const saving = ref<string>("")

const domain = ref<Domain | null>(null)
const clusters = ref<Cluster[]>([])
const certs = ref<Certificate[]>([])

const activeTab = ref("basic")
const httpAccessEnabled = ref(true)
const httpAccessPortBackup = ref(0)

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
  signed_url_secret: string
  bot_score_enabled: boolean
  response_compress_enabled: boolean
  edge_script_enabled: boolean
  edge_script_rules: string
  image_transform_enabled: boolean
  video_segment_cache_enabled: boolean
}>({
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
  signed_url_secret: "",
  bot_score_enabled: false,
  response_compress_enabled: false,
  edge_script_enabled: false,
  edge_script_rules: "",
  image_transform_enabled: false,
  video_segment_cache_enabled: false,
})
// "更多设置" 的展开状态由 <el-collapse v-model> 维护，默认收起。
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
const secModeToLegacyCC = (mode: string): { level: string; action: string; captcha_type?: string } => {
  switch (mode) {
    case "off":
      return { level: "low", action: "challenge" }
    case "loose":
    case "js":
      return { level: "low", action: "challenge", captcha_type: "js_challenge" }
    case "click":
    case "click_easy":
      return { level: "medium", action: "challenge", captcha_type: "click" }
    case "slide":
    case "slide_easy":
      return { level: "medium", action: "challenge", captcha_type: "slide" }
    case "captcha":
      return { level: "medium", action: "challenge", captcha_type: "slide_region" }
    case "rotate":
      return { level: "medium", action: "challenge", captcha_type: "rotate" }
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
      h(ElSwitch, {
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
        h(ElButton, { size: "small",
            type: "primary",
            link: true,
            onClick: () => openCustomRule(row),
          },
          () => "编辑",
        ),
        h(ElButton, { size: "small",
            type: "danger",
            link: true,
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
  error.value = ""
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
      await ensureTabData(activeTab.value, hit.id)
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

// Secondary-tab data is loaded lazily per tab group to reduce first-paint
// API pressure. Each sub-fetch is independently try/catch'd so a backend
// quirk doesn't paint the whole page broken.
const loadedTabGroups = reactive({
  domainFull: false,
  cache: false,
  security: false,
})

const loadDomainFull = async (domainId: string) => {
  try {
    const full = await api.getDomain(domainId)
    if (full) {
      domain.value = { ...domain.value, ...full }
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
}

const loadCacheTabData = async () => {
  try {
    const res = await api.listCacheRules()
    cacheRules.value = res.cache_rules || []
  } catch {
    cacheRules.value = []
  }
}

const loadSecurityTabData = async (domainId: string) => {
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
    const sec = await api.getDomainSecurity(domainId)
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
    secForm.signed_url_secret = sec.signed_url_secret || ""
    secForm.bot_score_enabled = Boolean(sec.bot_score_enabled)
    secForm.response_compress_enabled = Boolean(sec.response_compress_enabled)
    secForm.edge_script_enabled = Boolean(sec.edge_script_enabled)
    secForm.edge_script_rules = sec.edge_script_rules || ""
    secForm.image_transform_enabled = Boolean(sec.image_transform_enabled)
    secForm.video_segment_cache_enabled = Boolean(sec.video_segment_cache_enabled)
    hydrateRegionBlock(sec.blocked_regions || [])
  } catch {
    // first-save case: no security row yet
  }
  try {
    const res = await api.listWAFWhitelist()
    whitelistEntries.value = res.whitelist || []
  } catch {
    whitelistEntries.value = []
  }
}

const ensureTabData = async (tab: string, domainId: string) => {
  if (["basic", "origin", "https", "advanced"].includes(tab) && !loadedTabGroups.domainFull) {
    await loadDomainFull(domainId)
    loadedTabGroups.domainFull = true
  }
  if (tab === "cache" && !loadedTabGroups.cache) {
    await loadCacheTabData()
    loadedTabGroups.cache = true
  }
  if (["security", "access"].includes(tab) && !loadedTabGroups.security) {
    await loadSecurityTabData(domainId)
    loadedTabGroups.security = true
  }
}

const resetLoadedTabGroups = () => {
  loadedTabGroups.domainFull = false
  loadedTabGroups.cache = false
  loadedTabGroups.security = false
}

// Kept for any call sites that still expect a full secondary refresh.
const loadSecondary = async (domainId: string) => {
  resetLoadedTabGroups()
  await ensureTabData("basic", domainId)
  await ensureTabData("cache", domainId)
  await ensureTabData("security", domainId)
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
  httpAccessEnabled.value = true
  httpAccessPortBackup.value = d.listen_port ?? 0
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
  httpAccessEnabled.value = true
  httpAccessPortBackup.value = form.listen_port
}

watch(httpAccessEnabled, (on) => {
  if (on) {
    if (!form.listen_port && httpAccessPortBackup.value) {
      form.listen_port = httpAccessPortBackup.value
    }
    return
  }
  httpAccessPortBackup.value = form.listen_port || 80
  form.listen_port = 0
})

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
      { publish: false },
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
      signed_url_secret: secForm.signed_url_secret.trim(),
      bot_score_enabled: secForm.bot_score_enabled,
      response_compress_enabled: secForm.response_compress_enabled,
      edge_script_enabled: secForm.edge_script_enabled,
      edge_script_rules: secForm.edge_script_rules.trim(),
      image_transform_enabled: secForm.image_transform_enabled,
      video_segment_cache_enabled: secForm.video_segment_cache_enabled,
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
          captcha_type: legacy.captcha_type,
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
    if (id) {
      await loadSecurityTabData(id)
    }
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
      h(ElSwitch, {
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
        h(ElButton, { size: "small", type: "primary", link: true, onClick: () => openCacheRuleDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => confirmDeleteCacheRule(row) }, () => "删除"),
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
const canEditDomain = computed(() => isAdmin.value || canDomainsWrite.value)
const canEditSecurity = computed(() => isAdmin.value || canWAFEdit.value)
const readonlyNotice = computed(() => {
  const parts: string[] = []
  if (!canEditDomain.value) parts.push("域名配置")
  if (!canEditSecurity.value) parts.push("安全/WAF 设置")
  return `当前分组对${parts.join("与")}为只读，保存按钮已隐藏。`
})

// Hero 健康摘要：把 4 个最关键的开关（HTTPS / 缓存 / 安全策略 / WS）
// 聚合成一组只读 chip，让用户进入页面就能扫到当前接入网站的整体健康度，
// 不必再点开每个 tab 才知道。
// `security` chip considers the policy effective when at least one
// sub-field is configured: a non-"off" CC mode, any custom rule, any
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

watch(activeTab, (tab) => {
  const id = domain.value?.id
  if (id) void ensureTabData(tab, id)
})

watch(() => route.params.id, () => {
  resetLoadedTabGroups()
  void load()
})

onMounted(load)
</script>

