package templates

import "fmt"

// WAF 滑块验证模板 - 包含 PoW、轨迹分析、多语言支持
const wafSlideTemplate = `<!DOCTYPE html>
<html lang="{{LANG}}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Security Verification</title>
  <link rel="stylesheet" href="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.css"/>
  <style>
    *{box-sizing:border-box}
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f7f9;color:#1d2129}
    .wrap{max-width:400px;margin:8vh auto;padding:24px}
    .card{background:#fff;border:1px solid #e5e6eb;border-radius:12px;padding:24px;box-shadow:0 2px 8px rgba(0,0,0,.04)}
    .title{font-size:18px;font-weight:600;margin:0 0 8px;text-align:center}
    .desc{font-size:13px;color:#86909c;margin:0 0 20px;text-align:center}
    #captcha-wrap{min-height:260px;display:flex;align-items:center;justify-content:center}
    .loading{color:#86909c;font-size:13px}
    .pow-status{font-size:12px;color:#86909c;text-align:center;margin-top:12px;min-height:18px}
    .pow-status.computing{color:#165dff}
    .pow-status.done{color:#00b42a}
    .pow-status.error{color:#f53f3f}
    .hint{font-size:11px;color:#c9cdd4;text-align:center;margin-top:16px}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">Security Verification</h1>
      <p class="desc" data-i18n="desc">Please complete the verification to continue</p>
      <div id="captcha-wrap"><span class="loading" data-i18n="loading">Loading...</span></div>
      <div id="pow-status" class="pow-status"></div>
      <div class="hint" data-i18n="hint">Powered by {{SYSTEM_NAME}} WAF</div>
    </div>
  </div>
  <script src="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.js"></script>
  <script>
(function(){
  var TOKEN = '{{TOKEN}}';
  var CAPTCHA_DATA = {{CAPTCHA_DATA}};
  var POW_DIFFICULTY = {{POW_DIFFICULTY}};
  var POW_CHALLENGE = '{{POW_CHALLENGE}}';
  var LANG = '{{LANG}}' || 'zh';

  var i18n = {
    zh: {title:'安全验证',desc:'请完成验证后继续访问',loading:'加载中...',hint:'由 {{SYSTEM_NAME}} WAF 提供保护',pow_computing:'正在计算工作量证明...',pow_done:'验证计算完成',pow_error:'计算失败，请刷新重试',verify_success:'验证成功，正在跳转...',verify_fail:'验证失败，请重试'},
    en: {title:'Security Verification',desc:'Please complete the verification to continue',loading:'Loading...',hint:'Powered by {{SYSTEM_NAME}} WAF',pow_computing:'Computing proof of work...',pow_done:'Computation complete',pow_error:'Computation failed, please refresh',verify_success:'Verified, redirecting...',verify_fail:'Verification failed, please retry'}
  };
  var t = i18n[LANG] || i18n.en;

  document.querySelectorAll('[data-i18n]').forEach(function(el){
    var key = el.getAttribute('data-i18n');
    if(t[key]) el.textContent = t[key];
  });

  var trajectory = [];
  var powResult = null;
  var powStatus = document.getElementById('pow-status');

  // PoW 计算
  function computePoW(challenge, difficulty) {
    return new Promise(function(resolve, reject) {
      if (!difficulty || difficulty < 1) { resolve({nonce:0,hash:''}); return; }
      powStatus.className = 'pow-status computing';
      powStatus.textContent = t.pow_computing;
      var nonce = 0;
      var maxIter = 1e7;
      function work() {
        var batch = 1000;
        for(var i=0; i<batch && nonce<maxIter; i++, nonce++) {
          var data = challenge + ':' + nonce;
          var hash = simpleHash(data);
          if(hash.substring(0, difficulty) === '0'.repeat(difficulty)) {
            powStatus.className = 'pow-status done';
            powStatus.textContent = t.pow_done;
            resolve({nonce:nonce, hash:hash});
            return;
          }
        }
        if(nonce >= maxIter) {
          powStatus.className = 'pow-status error';
          powStatus.textContent = t.pow_error;
          reject(new Error('PoW timeout'));
        } else {
          setTimeout(work, 0);
        }
      }
      work();
    });
  }

  function simpleHash(str) {
    var hash = 0x811c9dc5;
    for(var i=0; i<str.length; i++) {
      hash ^= str.charCodeAt(i);
      hash = (hash * 0x01000193) >>> 0;
    }
    return hash.toString(16).padStart(8,'0');
  }

  // 轨迹记录
  function recordTrajectory(x, y) {
    trajectory.push({x:x, y:y, t:Date.now()});
    if(trajectory.length > 200) trajectory.shift();
  }

  // 初始化验证码
  function initCaptcha() {
    var el = document.getElementById('captcha-wrap');
    el.innerHTML = '';
    var capt = new GoCaptcha.Slide({width:300, height:220});
    capt.mount(el);
    capt.setData(CAPTCHA_DATA);
    capt.setEvents({
      move: function(x, y) { recordTrajectory(x, y); },
      confirm: function(point, reset) {
        submitVerification(point, reset);
        return false;
      },
      refresh: function() { location.reload(); },
      close: function() {}
    });
  }

  function submitVerification(point, reset) {
    var payload = {
      token: TOKEN,
      type: 'slide',
      point: point,
      trajectory: trajectory.slice(-50),
      pow: powResult
    };
    fetch(location.pathname + '?_waf_verify=1', {
      method: 'POST',
      headers: {'Content-Type':'application/json','X-WAF-Token':TOKEN},
      body: JSON.stringify(payload)
    }).then(function(r){ return r.json(); })
    .then(function(data){
      if(data.ok) {
        powStatus.className = 'pow-status done';
        powStatus.textContent = t.verify_success;
        setTimeout(function(){ location.reload(); }, 500);
      } else {
        powStatus.className = 'pow-status error';
        powStatus.textContent = t.verify_fail;
        reset && reset();
      }
    }).catch(function(){
      powStatus.className = 'pow-status error';
      powStatus.textContent = t.verify_fail;
      reset && reset();
    });
  }

  // 启动
  computePoW(POW_CHALLENGE, POW_DIFFICULTY)
    .then(function(r){ powResult = r; initCaptcha(); })
    .catch(function(){ initCaptcha(); });
})();
  </script>
</body>
</html>`

// WAF 点选验证模板
const wafClickTemplate = `<!DOCTYPE html>
<html lang="{{LANG}}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Security Verification</title>
  <link rel="stylesheet" href="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.css"/>
  <style>
    *{box-sizing:border-box}
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f7f9;color:#1d2129}
    .wrap{max-width:400px;margin:8vh auto;padding:24px}
    .card{background:#fff;border:1px solid #e5e6eb;border-radius:12px;padding:24px;box-shadow:0 2px 8px rgba(0,0,0,.04)}
    .title{font-size:18px;font-weight:600;margin:0 0 8px;text-align:center}
    .desc{font-size:13px;color:#86909c;margin:0 0 20px;text-align:center}
    #captcha-wrap{min-height:280px;display:flex;align-items:center;justify-content:center}
    .loading{color:#86909c;font-size:13px}
    .pow-status{font-size:12px;color:#86909c;text-align:center;margin-top:12px;min-height:18px}
    .pow-status.computing{color:#165dff}
    .pow-status.done{color:#00b42a}
    .pow-status.error{color:#f53f3f}
    .hint{font-size:11px;color:#c9cdd4;text-align:center;margin-top:16px}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">Security Verification</h1>
      <p class="desc" data-i18n="desc_click">Click the characters in order</p>
      <div id="captcha-wrap"><span class="loading" data-i18n="loading">Loading...</span></div>
      <div id="pow-status" class="pow-status"></div>
      <div class="hint" data-i18n="hint">Powered by {{SYSTEM_NAME}} WAF</div>
    </div>
  </div>
  <script src="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.js"></script>
  <script>
(function(){
  var TOKEN = '{{TOKEN}}';
  var CAPTCHA_DATA = {{CAPTCHA_DATA}};
  var POW_DIFFICULTY = {{POW_DIFFICULTY}};
  var POW_CHALLENGE = '{{POW_CHALLENGE}}';
  var LANG = '{{LANG}}' || 'zh';

  var i18n = {
    zh: {title:'安全验证',desc_click:'请按顺序点击图中文字',loading:'加载中...',hint:'由 {{SYSTEM_NAME}} WAF 提供保护',pow_computing:'正在计算...',pow_done:'计算完成',pow_error:'计算失败',verify_success:'验证成功',verify_fail:'验证失败，请重试'},
    en: {title:'Security Verification',desc_click:'Click the characters in order',loading:'Loading...',hint:'Powered by {{SYSTEM_NAME}} WAF',pow_computing:'Computing...',pow_done:'Complete',pow_error:'Failed',verify_success:'Verified',verify_fail:'Failed, please retry'}
  };
  var t = i18n[LANG] || i18n.en;
  document.querySelectorAll('[data-i18n]').forEach(function(el){
    var key = el.getAttribute('data-i18n');
    if(t[key]) el.textContent = t[key];
  });

  var trajectory = [], powResult = null;
  var powStatus = document.getElementById('pow-status');

  function computePoW(challenge, difficulty) {
    return new Promise(function(resolve, reject) {
      if (!difficulty || difficulty < 1) { resolve({nonce:0,hash:''}); return; }
      powStatus.className = 'pow-status computing';
      powStatus.textContent = t.pow_computing;
      var nonce = 0, maxIter = 1e7;
      function work() {
        for(var i=0; i<1000 && nonce<maxIter; i++, nonce++) {
          var hash = simpleHash(challenge + ':' + nonce);
          if(hash.substring(0, difficulty) === '0'.repeat(difficulty)) {
            powStatus.className = 'pow-status done';
            powStatus.textContent = t.pow_done;
            resolve({nonce:nonce, hash:hash}); return;
          }
        }
        if(nonce >= maxIter) { reject(new Error('timeout')); }
        else { setTimeout(work, 0); }
      }
      work();
    });
  }

  function simpleHash(str) {
    var hash = 0x811c9dc5;
    for(var i=0; i<str.length; i++) { hash ^= str.charCodeAt(i); hash = (hash * 0x01000193) >>> 0; }
    return hash.toString(16).padStart(8,'0');
  }

  function initCaptcha() {
    var el = document.getElementById('captcha-wrap');
    el.innerHTML = '';
    var capt = new GoCaptcha.Click({width:300, height:220});
    capt.mount(el);
    capt.setData(CAPTCHA_DATA);
    capt.setEvents({
      click: function(x, y) { trajectory.push({x:x, y:y, t:Date.now()}); },
      confirm: function(dots, reset) {
        submitVerification(dots, reset);
        return false;
      },
      refresh: function() { location.reload(); },
      close: function() {}
    });
  }

  function submitVerification(dots, reset) {
    fetch(location.pathname + '?_waf_verify=1', {
      method: 'POST',
      headers: {'Content-Type':'application/json','X-WAF-Token':TOKEN},
      body: JSON.stringify({token:TOKEN,type:'click',dots:dots,trajectory:trajectory.slice(-50),pow:powResult})
    }).then(function(r){ return r.json(); })
    .then(function(data){
      if(data.ok) { powStatus.textContent = t.verify_success; setTimeout(function(){ location.reload(); }, 500); }
      else { powStatus.className = 'pow-status error'; powStatus.textContent = t.verify_fail; reset && reset(); }
    }).catch(function(){ powStatus.textContent = t.verify_fail; reset && reset(); });
  }

  computePoW(POW_CHALLENGE, POW_DIFFICULTY).then(function(r){ powResult = r; initCaptcha(); }).catch(function(){ initCaptcha(); });
})();
  </script>
</body>
</html>`

// WAF 旋转验证模板
const wafRotateTemplate = `<!DOCTYPE html>
<html lang="{{LANG}}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Security Verification</title>
  <link rel="stylesheet" href="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.css"/>
  <style>
    *{box-sizing:border-box}
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f7f9;color:#1d2129}
    .wrap{max-width:400px;margin:8vh auto;padding:24px}
    .card{background:#fff;border:1px solid #e5e6eb;border-radius:12px;padding:24px;box-shadow:0 2px 8px rgba(0,0,0,.04)}
    .title{font-size:18px;font-weight:600;margin:0 0 8px;text-align:center}
    .desc{font-size:13px;color:#86909c;margin:0 0 20px;text-align:center}
    #captcha-wrap{min-height:280px;display:flex;align-items:center;justify-content:center}
    .loading{color:#86909c;font-size:13px}
    .pow-status{font-size:12px;color:#86909c;text-align:center;margin-top:12px;min-height:18px}
    .pow-status.computing{color:#165dff}
    .pow-status.done{color:#00b42a}
    .pow-status.error{color:#f53f3f}
    .hint{font-size:11px;color:#c9cdd4;text-align:center;margin-top:16px}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">Security Verification</h1>
      <p class="desc" data-i18n="desc_rotate">Rotate the image to the correct position</p>
      <div id="captcha-wrap"><span class="loading" data-i18n="loading">Loading...</span></div>
      <div id="pow-status" class="pow-status"></div>
      <div class="hint" data-i18n="hint">Powered by {{SYSTEM_NAME}} WAF</div>
    </div>
  </div>
  <script src="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.js"></script>
  <script>
(function(){
  var TOKEN = '{{TOKEN}}';
  var CAPTCHA_DATA = {{CAPTCHA_DATA}};
  var POW_DIFFICULTY = {{POW_DIFFICULTY}};
  var POW_CHALLENGE = '{{POW_CHALLENGE}}';
  var LANG = '{{LANG}}' || 'zh';

  var i18n = {
    zh: {title:'安全验证',desc_rotate:'请旋转图片至正确位置',loading:'加载中...',hint:'由 {{SYSTEM_NAME}} WAF 提供保护',pow_computing:'正在计算...',pow_done:'计算完成',verify_success:'验证成功',verify_fail:'验证失败，请重试'},
    en: {title:'Security Verification',desc_rotate:'Rotate the image to the correct position',loading:'Loading...',hint:'Powered by {{SYSTEM_NAME}} WAF',pow_computing:'Computing...',pow_done:'Complete',verify_success:'Verified',verify_fail:'Failed, please retry'}
  };
  var t = i18n[LANG] || i18n.en;
  document.querySelectorAll('[data-i18n]').forEach(function(el){
    var key = el.getAttribute('data-i18n');
    if(t[key]) el.textContent = t[key];
  });

  var powResult = null;
  var powStatus = document.getElementById('pow-status');

  function computePoW(challenge, difficulty) {
    return new Promise(function(resolve, reject) {
      if (!difficulty || difficulty < 1) { resolve({nonce:0,hash:''}); return; }
      powStatus.className = 'pow-status computing';
      powStatus.textContent = t.pow_computing;
      var nonce = 0, maxIter = 1e7;
      function work() {
        for(var i=0; i<1000 && nonce<maxIter; i++, nonce++) {
          var hash = simpleHash(challenge + ':' + nonce);
          if(hash.substring(0, difficulty) === '0'.repeat(difficulty)) {
            powStatus.className = 'pow-status done';
            powStatus.textContent = t.pow_done;
            resolve({nonce:nonce, hash:hash}); return;
          }
        }
        if(nonce >= maxIter) { reject(new Error('timeout')); }
        else { setTimeout(work, 0); }
      }
      work();
    });
  }

  function simpleHash(str) {
    var hash = 0x811c9dc5;
    for(var i=0; i<str.length; i++) { hash ^= str.charCodeAt(i); hash = (hash * 0x01000193) >>> 0; }
    return hash.toString(16).padStart(8,'0');
  }

  function initCaptcha() {
    var el = document.getElementById('captcha-wrap');
    el.innerHTML = '';
    var capt = new GoCaptcha.Rotate({width:300, height:220});
    capt.mount(el);
    capt.setData(CAPTCHA_DATA);
    capt.setEvents({
      confirm: function(angle, reset) {
        submitVerification(angle, reset);
        return false;
      },
      refresh: function() { location.reload(); },
      close: function() {}
    });
  }

  function submitVerification(angle, reset) {
    fetch(location.pathname + '?_waf_verify=1', {
      method: 'POST',
      headers: {'Content-Type':'application/json','X-WAF-Token':TOKEN},
      body: JSON.stringify({token:TOKEN,type:'rotate',angle:angle,pow:powResult})
    }).then(function(r){ return r.json(); })
    .then(function(data){
      if(data.ok) { powStatus.textContent = t.verify_success; setTimeout(function(){ location.reload(); }, 500); }
      else { powStatus.className = 'pow-status error'; powStatus.textContent = t.verify_fail; reset && reset(); }
    }).catch(function(){ powStatus.textContent = t.verify_fail; reset && reset(); });
  }

  computePoW(POW_CHALLENGE, POW_DIFFICULTY).then(function(r){ powResult = r; initCaptcha(); }).catch(function(){ initCaptcha(); });
})();
  </script>
</body>
</html>`

// WAF 拼图验证模板
const wafSlideRegionTemplate = `<!DOCTYPE html>
<html lang="{{LANG}}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Security Verification</title>
  <link rel="stylesheet" href="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.css"/>
  <style>
    *{box-sizing:border-box}
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f7f9;color:#1d2129}
    .wrap{max-width:400px;margin:8vh auto;padding:24px}
    .card{background:#fff;border:1px solid #e5e6eb;border-radius:12px;padding:24px;box-shadow:0 2px 8px rgba(0,0,0,.04)}
    .title{font-size:18px;font-weight:600;margin:0 0 8px;text-align:center}
    .desc{font-size:13px;color:#86909c;margin:0 0 20px;text-align:center}
    #captcha-wrap{min-height:280px;display:flex;align-items:center;justify-content:center}
    .loading{color:#86909c;font-size:13px}
    .pow-status{font-size:12px;color:#86909c;text-align:center;margin-top:12px;min-height:18px}
    .pow-status.computing{color:#165dff}
    .pow-status.done{color:#00b42a}
    .pow-status.error{color:#f53f3f}
    .hint{font-size:11px;color:#c9cdd4;text-align:center;margin-top:16px}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">Security Verification</h1>
      <p class="desc" data-i18n="desc_region">Drag the puzzle piece to the correct position</p>
      <div id="captcha-wrap"><span class="loading" data-i18n="loading">Loading...</span></div>
      <div id="pow-status" class="pow-status"></div>
      <div class="hint" data-i18n="hint">Powered by {{SYSTEM_NAME}} WAF</div>
    </div>
  </div>
  <script src="https://unpkg.com/go-captcha-jslib@1.0.9/dist/gocaptcha.global.js"></script>
  <script>
(function(){
  var TOKEN = '{{TOKEN}}';
  var CAPTCHA_DATA = {{CAPTCHA_DATA}};
  var POW_DIFFICULTY = {{POW_DIFFICULTY}};
  var POW_CHALLENGE = '{{POW_CHALLENGE}}';
  var LANG = '{{LANG}}' || 'zh';

  var i18n = {
    zh: {title:'安全验证',desc_region:'请拖动拼图至正确位置',loading:'加载中...',hint:'由 {{SYSTEM_NAME}} WAF 提供保护',pow_computing:'正在计算...',pow_done:'计算完成',verify_success:'验证成功',verify_fail:'验证失败，请重试'},
    en: {title:'Security Verification',desc_region:'Drag the puzzle piece to the correct position',loading:'Loading...',hint:'Powered by {{SYSTEM_NAME}} WAF',pow_computing:'Computing...',pow_done:'Complete',verify_success:'Verified',verify_fail:'Failed, please retry'}
  };
  var t = i18n[LANG] || i18n.en;
  document.querySelectorAll('[data-i18n]').forEach(function(el){
    var key = el.getAttribute('data-i18n');
    if(t[key]) el.textContent = t[key];
  });

  var trajectory = [], powResult = null;
  var powStatus = document.getElementById('pow-status');

  function computePoW(challenge, difficulty) {
    return new Promise(function(resolve, reject) {
      if (!difficulty || difficulty < 1) { resolve({nonce:0,hash:''}); return; }
      powStatus.className = 'pow-status computing';
      powStatus.textContent = t.pow_computing;
      var nonce = 0, maxIter = 1e7;
      function work() {
        for(var i=0; i<1000 && nonce<maxIter; i++, nonce++) {
          var hash = simpleHash(challenge + ':' + nonce);
          if(hash.substring(0, difficulty) === '0'.repeat(difficulty)) {
            powStatus.className = 'pow-status done';
            powStatus.textContent = t.pow_done;
            resolve({nonce:nonce, hash:hash}); return;
          }
        }
        if(nonce >= maxIter) { reject(new Error('timeout')); }
        else { setTimeout(work, 0); }
      }
      work();
    });
  }

  function simpleHash(str) {
    var hash = 0x811c9dc5;
    for(var i=0; i<str.length; i++) { hash ^= str.charCodeAt(i); hash = (hash * 0x01000193) >>> 0; }
    return hash.toString(16).padStart(8,'0');
  }

  function initCaptcha() {
    var el = document.getElementById('captcha-wrap');
    el.innerHTML = '';
    var capt = new GoCaptcha.SlideRegion({width:300, height:220});
    capt.mount(el);
    capt.setData(CAPTCHA_DATA);
    capt.setEvents({
      move: function(x, y) { trajectory.push({x:x, y:y, t:Date.now()}); },
      confirm: function(point, reset) {
        submitVerification(point, reset);
        return false;
      },
      refresh: function() { location.reload(); },
      close: function() {}
    });
  }

  function submitVerification(point, reset) {
    fetch(location.pathname + '?_waf_verify=1', {
      method: 'POST',
      headers: {'Content-Type':'application/json','X-WAF-Token':TOKEN},
      body: JSON.stringify({token:TOKEN,type:'slide_region',point:point,trajectory:trajectory.slice(-50),pow:powResult})
    }).then(function(r){ return r.json(); })
    .then(function(data){
      if(data.ok) { powStatus.textContent = t.verify_success; setTimeout(function(){ location.reload(); }, 500); }
      else { powStatus.className = 'pow-status error'; powStatus.textContent = t.verify_fail; reset && reset(); }
    }).catch(function(){ powStatus.textContent = t.verify_fail; reset && reset(); });
  }

  computePoW(POW_CHALLENGE, POW_DIFFICULTY).then(function(r){ powResult = r; initCaptcha(); }).catch(function(){ initCaptcha(); });
})();
  </script>
</body>
</html>`

// WAF 无感验证模板 - JS Challenge + PoW
const wafJsChallengeTemplate = `<!DOCTYPE html>
<html lang="{{LANG}}">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width,initial-scale=1"/>
  <title>Security Check</title>
  <style>
    *{box-sizing:border-box}
    body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;background:#f5f7f9;color:#1d2129;display:flex;align-items:center;justify-content:center;min-height:100vh}
    .wrap{max-width:360px;width:100%;padding:24px;text-align:center}
    .card{background:#fff;border:1px solid #e5e6eb;border-radius:12px;padding:32px 24px;box-shadow:0 2px 8px rgba(0,0,0,.04)}
    .spinner{width:40px;height:40px;margin:0 auto 16px;border:3px solid #e5e6eb;border-top-color:#165dff;border-radius:50%;animation:spin 1s linear infinite}
    @keyframes spin{to{transform:rotate(360deg)}}
    .title{font-size:16px;font-weight:500;margin:0 0 8px}
    .desc{font-size:13px;color:#86909c;margin:0}
    .status{font-size:12px;color:#86909c;margin-top:16px;min-height:18px}
    .status.success{color:#00b42a}
    .status.error{color:#f53f3f}
    .hint{font-size:11px;color:#c9cdd4;margin-top:20px}
    .retry{display:none;margin-top:16px;padding:8px 20px;background:#165dff;color:#fff;border:none;border-radius:6px;font-size:13px;cursor:pointer}
    .retry:hover{background:#4080ff}
    .retry.show{display:inline-block}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <div class="spinner" id="spinner"></div>
      <h1 class="title" id="title">Checking your browser...</h1>
      <p class="desc" id="desc">This process is automatic</p>
      <div class="status" id="status"></div>
      <button class="retry" id="retry" onclick="location.reload()">Retry</button>
    </div>
    <div class="hint" data-i18n="hint">Powered by {{SYSTEM_NAME}} WAF</div>
  </div>
  <script>
(function(){
  var TOKEN = '{{TOKEN}}';
  var POW_DIFFICULTY = {{POW_DIFFICULTY}};
  var POW_CHALLENGE = '{{POW_CHALLENGE}}';
  var LANG = '{{LANG}}' || 'zh';

  var i18n = {
    zh: {title:'正在检查您的浏览器...',desc:'此过程自动完成',computing:'正在验证...',success:'验证通过，正在跳转...',error:'验证失败',hint:'由 {{SYSTEM_NAME}} WAF 提供保护',retry:'重试'},
    en: {title:'Checking your browser...',desc:'This process is automatic',computing:'Verifying...',success:'Verified, redirecting...',error:'Verification failed',hint:'Powered by {{SYSTEM_NAME}} WAF',retry:'Retry'}
  };
  var t = i18n[LANG] || i18n.en;

  document.getElementById('title').textContent = t.title;
  document.getElementById('desc').textContent = t.desc;
  document.getElementById('retry').textContent = t.retry;
  document.querySelector('[data-i18n="hint"]').textContent = t.hint;

  var status = document.getElementById('status');
  var spinner = document.getElementById('spinner');
  var retry = document.getElementById('retry');

  // 收集浏览器指纹
  function collectFingerprint() {
    var fp = {
      ua: navigator.userAgent,
      lang: navigator.language,
      platform: navigator.platform,
      cores: navigator.hardwareConcurrency || 0,
      memory: navigator.deviceMemory || 0,
      screen: screen.width + 'x' + screen.height,
      tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
      touch: 'ontouchstart' in window,
      webgl: getWebGLRenderer(),
      canvas: getCanvasFingerprint(),
      ts: Date.now()
    };
    return fp;
  }

  function getWebGLRenderer() {
    try {
      var canvas = document.createElement('canvas');
      var gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
      if (gl) {
        var ext = gl.getExtension('WEBGL_debug_renderer_info');
        if (ext) return gl.getParameter(ext.UNMASKED_RENDERER_WEBGL);
      }
    } catch(e) {}
    return '';
  }

  function getCanvasFingerprint() {
    try {
      var canvas = document.createElement('canvas');
      var ctx = canvas.getContext('2d');
      ctx.textBaseline = 'top';
      ctx.font = '14px Arial';
      ctx.fillText('LingCDN', 2, 2);
      return canvas.toDataURL().slice(-50);
    } catch(e) {}
    return '';
  }

  // PoW 计算
  function computePoW(challenge, difficulty) {
    return new Promise(function(resolve, reject) {
      if (!difficulty || difficulty < 1) { resolve({nonce:0,hash:''}); return; }
      status.textContent = t.computing;
      var nonce = 0, maxIter = 1e7;
      function work() {
        for(var i=0; i<2000 && nonce<maxIter; i++, nonce++) {
          var hash = simpleHash(challenge + ':' + nonce);
          if(hash.substring(0, difficulty) === '0'.repeat(difficulty)) {
            resolve({nonce:nonce, hash:hash}); return;
          }
        }
        if(nonce >= maxIter) { reject(new Error('timeout')); }
        else { setTimeout(work, 0); }
      }
      work();
    });
  }

  function simpleHash(str) {
    var hash = 0x811c9dc5;
    for(var i=0; i<str.length; i++) { hash ^= str.charCodeAt(i); hash = (hash * 0x01000193) >>> 0; }
    return hash.toString(16).padStart(8,'0');
  }

  // 提交验证
  function submit(pow, fp) {
    fetch(location.pathname + '?_waf_verify=1', {
      method: 'POST',
      headers: {'Content-Type':'application/json','X-WAF-Token':TOKEN},
      body: JSON.stringify({token:TOKEN,type:'js_challenge',pow:pow,fingerprint:fp})
    }).then(function(r){ return r.json(); })
    .then(function(data){
      if(data.ok) {
        spinner.style.borderTopColor = '#00b42a';
        status.className = 'status success';
        status.textContent = t.success;
        setTimeout(function(){ location.reload(); }, 500);
      } else { showError(); }
    }).catch(showError);
  }

  function showError() {
    spinner.style.display = 'none';
    status.className = 'status error';
    status.textContent = t.error;
    retry.className = 'retry show';
  }

  // 启动
  var fp = collectFingerprint();
  computePoW(POW_CHALLENGE, POW_DIFFICULTY)
    .then(function(pow){ submit(pow, fp); })
    .catch(showError);
})();
  </script>
</body>
</html>`

type GlobalTemplateDef struct {
	Key            string
	Name           string
	Group          string
	Mode           string
	DefaultContent string
	Placeholders   []string
}

func DefaultGlobalTemplateDefs() []GlobalTemplateDef {
	return []GlobalTemplateDef{
		{
			Key:   "waf.ban.page",
			Name:  "节点禁止IP访问",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{"{{SYSTEM_NAME}}"},
			DefaultContent: `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">访问被拦截</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 760px; margin: 10vh auto; padding: 24px; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 22px; }
    .title { font-size: 20px; font-weight: 700; margin: 0 0 8px; }
    .desc { font-size: 14px; color: #4e5969; margin: 0; line-height: 1.6; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">访问被拦截</h1>
      <p class="desc" data-i18n="desc">您的访问已被安全策略拦截。如需协助请联系站点管理员。</p>
      <div class="footer" data-i18n="footer">由 {{SYSTEM_NAME}} 提供保护</div>
    </div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'访问被拦截', desc:'您的访问已被安全策略拦截。如需协助请联系站点管理员。', footer:'由 {{SYSTEM_NAME}} 提供保护'};
    var EN = {title:'Access Blocked', desc:'Your request has been blocked by our security policy. Please contact the site administrator if you need assistance.', footer:'Protected by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`,
		},
		{
			Key:   "waf.challenge.page",
			Name:  "WAF 通用消息提示界面",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{QUESTION}}",
				"{{WAIT_SECONDS}}",
				"{{SYSTEM_NAME}}",
			},
			DefaultContent: `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">安全验证</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 760px; margin: 10vh auto; padding: 24px; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 22px; }
    .title { font-size: 20px; font-weight: 700; margin: 0 0 8px; }
    .desc { font-size: 14px; color: #4e5969; margin: 0 0 16px; line-height: 1.6; }
    .row { display:flex; gap: 12px; flex-wrap: wrap; align-items: center; }
    input { height: 36px; padding: 0 12px; border: 1px solid #dcdfe6; border-radius: 8px; font-size: 14px; outline: none; }
    button { height: 36px; padding: 0 14px; border: 0; border-radius: 8px; background: #165dff; color:#fff; font-weight: 600; cursor: pointer; }
    button:disabled { background: #a5c3ff; cursor: not-allowed; }
    .hint { margin-top: 12px; font-size: 12px; color: #86909c; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">安全验证</h1>
      <p class="desc" data-i18n="desc">为了保护站点安全，请完成验证后继续访问。</p>
      <div class="row">
        <span data-i18n="question" style="font-size:14px;color:#1d2129;">{{QUESTION}}</span>
        <input id="ans" data-i18n-attr="placeholder:answer_ph" placeholder="请输入答案" />
        <button id="btn" data-i18n="submit">提交</button>
      </div>
      <div class="hint" data-i18n="hint">提交后请等待 {{WAIT_SECONDS}} 秒后重试请求（客户端需携带请求头 X-WAF-Token / X-WAF-Answer）。</div>
      <div class="footer" data-i18n="footer">由 {{SYSTEM_NAME}} 提供保护</div>
    </div>
  </div>
  <script>
    (function(){
      var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
      var isZh = L.indexOf('zh') === 0;
      document.documentElement.lang = isZh ? 'zh-CN' : 'en';
      var ZH = {title:'安全验证', desc:'为了保护站点安全，请完成验证后继续访问。', question:'{{QUESTION}}', answer_ph:'请输入答案', submit:'提交', hint:'提交后请等待 {{WAIT_SECONDS}} 秒后重试请求（客户端需携带请求头 X-WAF-Token / X-WAF-Answer）。', footer:'由 {{SYSTEM_NAME}} 提供保护'};
      var EN = {title:'Security Verification', desc:'To protect the site, please complete the verification before continuing.', question:'{{QUESTION}}', answer_ph:'Enter your answer', submit:'Submit', hint:'After submitting, please wait {{WAIT_SECONDS}} seconds before retrying (the client must send the X-WAF-Token / X-WAF-Answer headers).', footer:'Protected by {{SYSTEM_NAME}}'};
      var t = isZh ? ZH : EN;
      if (t.title) document.title = t.title;
      document.querySelectorAll('[data-i18n]').forEach(function(el){
        var k = el.getAttribute('data-i18n');
        if (t[k]) el.textContent = t[k];
      });
      document.querySelectorAll('[data-i18n-attr]').forEach(function(el){
        var spec = el.getAttribute('data-i18n-attr') || '';
        spec.split(',').forEach(function(pair){
          var p = pair.split(':');
          if (p.length === 2 && t[p[1]]) el.setAttribute(p[0].trim(), t[p[1]].trim());
        });
      });

      var btn = document.getElementById('btn');
      var ans = document.getElementById('ans');
      btn.addEventListener('click', function(){
        btn.disabled = true;
        var token = '{{TOKEN}}';
        var answer = (ans && ans.value || '').trim();
        try { navigator.clipboard && navigator.clipboard.writeText('X-WAF-Token: ' + token + '\nX-WAF-Answer: ' + answer); } catch(e) {}
        btn.disabled = false;
      });
    })();
  </script>
</body>
</html>`,
		},
		{
			Key:   "error.502.html",
			Name:  "502 错误页面",
			Group: "错误页",
			Mode:  "html",
			Placeholders: []string{
				"{{status}}",
				"{{host}}",
				"{{path}}",
				"{{request_id}}",
				"{{SYSTEM_NAME}}",
			},
			DefaultContent: buildDefaultErrorPage(502, "网关错误", "Bad Gateway", "上游服务暂时不可用，请稍后重试。", "The upstream service is temporarily unavailable. Please try again later."),
		},
		{
			Key:   "error.504.html",
			Name:  "504 错误页面",
			Group: "错误页",
			Mode:  "html",
			Placeholders: []string{
				"{{status}}",
				"{{host}}",
				"{{path}}",
				"{{request_id}}",
				"{{SYSTEM_NAME}}",
			},
			DefaultContent: buildDefaultErrorPage(504, "网关超时", "Gateway Timeout", "上游服务响应超时，请稍后重试。", "The upstream service timed out. Please try again later."),
		},
		{
			Key:   "email.register_code.text",
			Name:  "邮件-注册邮件验证码",
			Group: "邮件",
			Mode:  "text",
			Placeholders: []string{
				"{{system_name}}",
				"{{code}}",
				"{{ttl_minutes}}",
				"{{email}}",
			},
			DefaultContent: `您好，

您正在注册 {{system_name}} 账号。
验证码：{{code}}
有效期：{{ttl_minutes}} 分钟

如非本人操作，请忽略此邮件。`,
		},
		// WAF 滑块验证模板
		{
			Key:   "waf.challenge.slide",
			Name:  "WAF 滑块验证",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{CAPTCHA_DATA}}",
				"{{POW_DIFFICULTY}}",
				"{{POW_CHALLENGE}}",
				"{{LANG}}",
			},
			DefaultContent: wafSlideTemplate,
		},
		// WAF 点选验证模板
		{
			Key:   "waf.challenge.click",
			Name:  "WAF 点选验证",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{CAPTCHA_DATA}}",
				"{{POW_DIFFICULTY}}",
				"{{POW_CHALLENGE}}",
				"{{LANG}}",
			},
			DefaultContent: wafClickTemplate,
		},
		// WAF 旋转验证模板
		{
			Key:   "waf.challenge.rotate",
			Name:  "WAF 旋转验证",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{CAPTCHA_DATA}}",
				"{{POW_DIFFICULTY}}",
				"{{POW_CHALLENGE}}",
				"{{LANG}}",
			},
			DefaultContent: wafRotateTemplate,
		},
		// WAF 拼图验证模板
		{
			Key:   "waf.challenge.slide_region",
			Name:  "WAF 拼图验证",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{CAPTCHA_DATA}}",
				"{{POW_DIFFICULTY}}",
				"{{POW_CHALLENGE}}",
				"{{LANG}}",
			},
			DefaultContent: wafSlideRegionTemplate,
		},
		// WAF 无感验证模板 (JS Challenge)
		{
			Key:   "waf.challenge.js",
			Name:  "WAF 无感验证",
			Group: "WAF",
			Mode:  "html",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{POW_DIFFICULTY}}",
				"{{POW_CHALLENGE}}",
				"{{LANG}}",
			},
			DefaultContent: wafJsChallengeTemplate,
		},
		{
			Key:           "error.400.html",
			Name:          "400 错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: buildDefaultErrorPage(400, "请求错误", "Bad Request", "请求格式或参数有误，请检查后重试。", "The request was malformed or invalid. Please check it and try again."),
		},
		{
			Key:           "error.403.html",
			Name:          "403 错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: buildDefaultErrorPage(403, "访问被拒绝", "Forbidden", "您无权访问该资源。", "You do not have permission to access this resource."),
		},
		{
			Key:           "error.404.html",
			Name:          "404 错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: buildDefaultErrorPage(404, "页面未找到", "Not Found", "您访问的页面不存在。", "The page you requested could not be found."),
		},
		{
			Key:           "error.500.html",
			Name:          "500 错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: buildDefaultErrorPage(500, "服务器内部错误", "Internal Server Error", "服务器发生了意料之外的错误，请稍后重试。", "The server encountered an unexpected error. Please try again later."),
		},
		{
			Key:           "error.503.html",
			Name:          "503 错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: buildDefaultErrorPage(503, "服务暂不可用", "Service Unavailable", "服务器暂时繁忙或在维护，请稍后重试。", "The server is busy or under maintenance. Please try again later."),
		},
		{
			Key:           "error.default.html",
			Name:          "默认错误页面",
			Group:         "错误页",
			Mode:          "html",
			Placeholders:  []string{"{{status}}", "{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: defaultErrorPageTemplate,
		},
		{
			Key:            "waf.shield.page",
			Name:           "WAF 挡墙等待页 (shield_5s)",
			Group:          "WAF",
			Mode:           "html",
			Placeholders:   []string{"{{WAIT_SECONDS}}", "{{SYSTEM_NAME}}"},
			DefaultContent: wafShieldPageTemplate,
		},
		{
			Key:            "waf.ban.default",
			Name:           "WAF拦截页",
			Group:          "WAF",
			Mode:           "html",
			Placeholders:   []string{"{{SYSTEM_NAME}}"},
			DefaultContent: wafBanDefaultTemplate,
		},
		{
			Key:   "waf.challenge.default_json",
			Name:  "WAF 默认 Challenge JSON",
			Group: "WAF",
			Mode:  "json",
			Placeholders: []string{
				"{{TOKEN}}",
				"{{QUESTION}}",
				"{{WAIT_SECONDS}}",
			},
			DefaultContent: wafChallengeDefaultJSON,
		},
		{
			Key:   "email.password_reset_code.text",
			Name:  "邮件-密码重置验证码",
			Group: "邮件",
			Mode:  "text",
			Placeholders: []string{
				"{{system_name}}",
				"{{code}}",
				"{{ttl_minutes}}",
				"{{email}}",
			},
			DefaultContent: `您好，

您正在重置 {{system_name}} 账号密码。
验证码：{{code}}
有效期：{{ttl_minutes}} 分钟

如非本人操作，请忽略此邮件。`,
		},
		{
			Key:          "node.cname_not_found",
			Name:         "CNAME 域名未找到页面",
			Group:        "节点页面",
			Mode:         "html",
			Placeholders: []string{"{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: nodeCnameNotFoundTemplate,
		},
		{
			Key:          "node.direct_ip",
			Name:         "直接访问节点 IP 页面",
			Group:        "节点页面",
			Mode:         "html",
			Placeholders: []string{"{{host}}", "{{path}}", "{{request_id}}", "{{SYSTEM_NAME}}"},
			DefaultContent: nodeDirectIPTemplate,
		},
	}
}

const nodeCnameNotFoundTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>站点未找到</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif;
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 24px;
    }
    .container {
      max-width: 520px;
      width: 100%;
    }
    .card {
      background: rgba(255,255,255,0.95);
      backdrop-filter: blur(20px);
      border-radius: 16px;
      padding: 40px 32px;
      box-shadow: 0 20px 60px rgba(0,0,0,0.15);
    }
    .icon {
      width: 64px;
      height: 64px;
      background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
      border-radius: 16px;
      display: flex;
      align-items: center;
      justify-content: center;
      margin-bottom: 24px;
      font-size: 28px;
    }
    .title {
      font-size: 22px;
      font-weight: 700;
      color: #1a1a2e;
      margin-bottom: 12px;
    }
    .desc {
      font-size: 15px;
      color: #4a4a6a;
      line-height: 1.7;
      margin-bottom: 24px;
    }
    .info-box {
      background: #f8f9fc;
      border: 1px solid #e8eaf0;
      border-radius: 10px;
      padding: 16px;
    }
    .info-row {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      font-size: 13px;
      color: #6b7280;
    }
    .info-row + .info-row { margin-top: 8px; }
    .info-label {
      color: #9ca3af;
      flex-shrink: 0;
      min-width: 70px;
    }
    .info-value {
      color: #374151;
      word-break: break-all;
    }
    .footer {
      text-align: center;
      margin-top: 20px;
      font-size: 12px;
      color: rgba(255,255,255,0.6);
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="card">
      <div class="icon">🔍</div>
      <h1 class="title" data-i18n="title">站点未找到</h1>
      <p class="desc"><span data-i18n="desc_pre">您访问的域名</span> <strong>{{host}}</strong> <span data-i18n="desc_post">尚未在本 CDN 平台配置，或已被停用。请检查域名的 CNAME 解析是否正确指向本平台。</span></p>
      <div class="info-box">
        <div class="info-row">
          <span class="info-label" data-i18n="label_host">域名</span>
          <span class="info-value">{{host}}</span>
        </div>
        <div class="info-row">
          <span class="info-label" data-i18n="label_path">路径</span>
          <span class="info-value">{{path}}</span>
        </div>
        <div class="info-row">
          <span class="info-label" data-i18n="label_req">请求编号</span>
          <span class="info-value">{{request_id}}</span>
        </div>
      </div>
    </div>
    <div class="footer" data-i18n="footer">Powered by {{SYSTEM_NAME}}</div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'站点未找到', desc_pre:'您访问的域名', desc_post:'尚未在本 CDN 平台配置，或已被停用。请检查域名的 CNAME 解析是否正确指向本平台。', label_host:'域名', label_path:'路径', label_req:'请求编号', footer:'Powered by {{SYSTEM_NAME}}'};
    var EN = {title:'Site not found', desc_pre:'The host', desc_post:'has not been configured on this CDN, or has been disabled. Please verify that the CNAME record points to this platform.', label_host:'Host', label_path:'Path', label_req:'Request ID', footer:'Powered by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`

const nodeDirectIPTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>CDN 节点</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif;
      background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 24px;
      overflow: hidden;
    }
    .container {
      max-width: 480px;
      width: 100%;
      position: relative;
      z-index: 1;
    }
    .card {
      background: rgba(255,255,255,0.06);
      backdrop-filter: blur(24px);
      border: 1px solid rgba(255,255,255,0.1);
      border-radius: 20px;
      padding: 48px 36px;
      text-align: center;
    }
    .shield {
      width: 72px;
      height: 72px;
      background: linear-gradient(135deg, #00d2ff 0%, #3a7bd5 100%);
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0 auto 28px;
      font-size: 32px;
      box-shadow: 0 8px 32px rgba(0,210,255,0.3);
    }
    .title {
      font-size: 24px;
      font-weight: 700;
      color: #ffffff;
      margin-bottom: 12px;
    }
    .desc {
      font-size: 15px;
      color: rgba(255,255,255,0.65);
      line-height: 1.7;
      margin-bottom: 32px;
    }
    .status-bar {
      display: flex;
      justify-content: center;
      gap: 32px;
      margin-bottom: 28px;
    }
    .status-item {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 6px;
    }
    .status-dot {
      width: 10px;
      height: 10px;
      border-radius: 50%;
      background: #00e676;
      box-shadow: 0 0 12px rgba(0,230,118,0.5);
      animation: pulse 2s infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.5; }
    }
    .status-label {
      font-size: 12px;
      color: rgba(255,255,255,0.5);
    }
    .status-value {
      font-size: 14px;
      color: rgba(255,255,255,0.85);
      font-weight: 500;
    }
    .info-box {
      background: rgba(255,255,255,0.05);
      border: 1px solid rgba(255,255,255,0.08);
      border-radius: 10px;
      padding: 14px 16px;
      text-align: left;
    }
    .info-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-size: 13px;
    }
    .info-row + .info-row { margin-top: 8px; }
    .info-label { color: rgba(255,255,255,0.4); }
    .info-value { color: rgba(255,255,255,0.75); font-family: "SF Mono", Menlo, monospace; }
    .footer {
      text-align: center;
      margin-top: 24px;
      font-size: 12px;
      color: rgba(255,255,255,0.25);
    }
    .bg-glow {
      position: fixed;
      width: 400px;
      height: 400px;
      border-radius: 50%;
      filter: blur(120px);
      opacity: 0.15;
      pointer-events: none;
    }
    .glow-1 { top: -100px; left: -100px; background: #667eea; }
    .glow-2 { bottom: -100px; right: -100px; background: #764ba2; }
  </style>
</head>
<body>
  <div class="bg-glow glow-1"></div>
  <div class="bg-glow glow-2"></div>
  <div class="container">
    <div class="card">
      <div class="shield">🛡️</div>
      <h1 class="title" data-i18n="title">CDN 节点运行中</h1>
      <p class="desc" data-i18n="desc">您正在直接访问 CDN 加速节点。本节点仅为已配置的域名提供加速服务，不支持通过 IP 直接访问。</p>
      <div class="status-bar">
        <div class="status-item">
          <div class="status-dot"></div>
          <span class="status-label" data-i18n="status_node">节点状态</span>
          <span class="status-value" data-i18n="running">运行中</span>
        </div>
        <div class="status-item">
          <div class="status-dot"></div>
          <span class="status-label" data-i18n="status_protect">防护状态</span>
          <span class="status-value" data-i18n="enabled">已启用</span>
        </div>
      </div>
      <div class="info-box">
        <div class="info-row">
          <span class="info-label" data-i18n="label_ip">访问 IP</span>
          <span class="info-value">{{host}}</span>
        </div>
        <div class="info-row">
          <span class="info-label" data-i18n="label_req">请求编号</span>
          <span class="info-value">{{request_id}}</span>
        </div>
      </div>
    </div>
    <div class="footer" data-i18n="footer">Powered by {{SYSTEM_NAME}}</div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'CDN 节点运行中', desc:'您正在直接访问 CDN 加速节点。本节点仅为已配置的域名提供加速服务，不支持通过 IP 直接访问。', status_node:'节点状态', running:'运行中', status_protect:'防护状态', enabled:'已启用', label_ip:'访问 IP', label_req:'请求编号', footer:'Powered by {{SYSTEM_NAME}}'};
    var EN = {title:'CDN node is online', desc:'You are accessing a CDN edge node directly. This node only serves traffic for configured domains and cannot be reached by IP.', status_node:'Node status', running:'Running', status_protect:'Protection', enabled:'Enabled', label_ip:'Client IP', label_req:'Request ID', footer:'Powered by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`

func buildDefaultErrorPage(status int, titleZh, titleEn, descZh, descEn string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">%d %s</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 760px; margin: 10vh auto; padding: 24px; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 22px; }
    .code { font-size: 42px; font-weight: 800; margin: 0; }
    .desc { margin: 8px 0 0; font-size: 14px; color: #4e5969; line-height: 1.6; }
    .meta { margin-top: 14px; font-size: 12px; color: #86909c; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <div class="code">%d</div>
      <div class="desc" data-i18n="desc">%s</div>
      <div class="meta">host={{host}} path={{path}} request_id={{request_id}}</div>
      <div class="footer" data-i18n="footer">Powered by {{SYSTEM_NAME}}</div>
    </div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'%d %s', desc:'%s', footer:'Powered by {{SYSTEM_NAME}}'};
    var EN = {title:'%d %s', desc:'%s', footer:'Powered by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`, status, titleZh, status, descZh, status, titleZh, descZh, status, titleEn, descEn)
}

const defaultErrorPageTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">{{status}}</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 760px; margin: 10vh auto; padding: 24px; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 22px; }
    .code { font-size: 42px; font-weight: 800; margin: 0; }
    .desc { margin: 8px 0 0; font-size: 14px; color: #4e5969; line-height: 1.6; }
    .meta { margin-top: 14px; font-size: 12px; color: #86909c; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <div class="code">{{status}}</div>
      <div class="desc" data-i18n="desc">请求未能成功完成。</div>
      <div class="meta">host={{host}} path={{path}} request_id={{request_id}}</div>
      <div class="footer" data-i18n="footer">Powered by {{SYSTEM_NAME}}</div>
    </div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'{{status}}', desc:'请求未能成功完成。', footer:'Powered by {{SYSTEM_NAME}}'};
    var EN = {title:'{{status}}', desc:'The request could not be completed.', footer:'Powered by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`

const wafShieldPageTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">请稍候</title>
  <meta http-equiv="refresh" content="{{WAIT_SECONDS}}" />
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 520px; margin: 15vh auto; padding: 24px; text-align: center; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 28px; }
    .title { font-size: 22px; font-weight: 700; margin: 0 0 10px; }
    .desc { font-size: 14px; color:#4e5969; margin:0 0 18px; line-height: 1.6; }
    .num { font-size: 36px; font-weight: 800; color: #165dff; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">请稍候</h1>
      <p class="desc"><span data-i18n="desc_pre">当前请求频率受到保护，请等待</span> <span class="num">{{WAIT_SECONDS}}</span> <span data-i18n="desc_post">秒后自动重试。</span></p>
      <div class="footer" data-i18n="footer">由 {{SYSTEM_NAME}} 提供保护</div>
    </div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'请稍候', desc_pre:'当前请求频率受到保护，请等待', desc_post:'秒后自动重试。', footer:'由 {{SYSTEM_NAME}} 提供保护'};
    var EN = {title:'Please wait', desc_pre:'Your request rate is being protected. Please wait', desc_post:'seconds, then we will retry automatically.', footer:'Protected by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`

const wafBanDefaultTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title data-i18n="title">访问被拒绝</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Arial, "PingFang SC", "Microsoft YaHei", sans-serif; background:#f5f7f9; color:#1d2129; }
    .wrap { max-width: 520px; margin: 15vh auto; padding: 24px; text-align: center; }
    .card { background:#fff; border: 1px solid #e5e6eb; border-radius: 10px; padding: 28px; }
    .title { font-size: 22px; font-weight: 700; margin: 0 0 10px; }
    .desc { font-size: 14px; color:#4e5969; margin:0; line-height: 1.6; }
    .footer { margin-top: 18px; font-size: 12px; color: #86909c; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1 class="title" data-i18n="title">访问被拒绝</h1>
      <p class="desc" data-i18n="desc">您的请求已被 WAF 安全策略拦截。</p>
      <div class="footer" data-i18n="footer">由 {{SYSTEM_NAME}} 提供保护</div>
    </div>
  </div>
  <script>
  (function(){
    var L = (navigator.language || navigator.userLanguage || '').toLowerCase();
    var isZh = L.indexOf('zh') === 0;
    document.documentElement.lang = isZh ? 'zh-CN' : 'en';
    var ZH = {title:'访问被拒绝', desc:'您的请求已被 WAF 安全策略拦截。', footer:'由 {{SYSTEM_NAME}} 提供保护'};
    var EN = {title:'Access Denied', desc:'Your request has been blocked by the WAF security policy.', footer:'Protected by {{SYSTEM_NAME}}'};
    var t = isZh ? ZH : EN;
    if (t.title) document.title = t.title;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k = el.getAttribute('data-i18n');
      if (t[k]) el.textContent = t[k];
    });
  })();
  </script>
</body>
</html>`

const wafChallengeDefaultJSON = `{
  "waf_challenge": true,
  "question": "{{QUESTION}}",
  "token": "{{TOKEN}}",
  "wait_seconds": {{WAIT_SECONDS}},
  "msg": "Captcha required"
}`

