package sync

import (
	"fmt"
)

const SignPrefix = "forgor-sync-v1"

func SignBytesDeviceBundle(deviceID, pubkeySign, pubkeyBox []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("device_bundle")
	if err := e.WriteDeviceID(deviceID); err != nil {
		return nil, fmt.Errorf("device_id: %w", err)
	}
	if err := e.WritePublicKey(pubkeySign); err != nil {
		return nil, fmt.Errorf("pubkey_sign: %w", err)
	}
	if err := e.WritePublicKey(pubkeyBox); err != nil {
		return nil, fmt.Errorf("pubkey_box: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesEvent(eventID, vaultID, deviceID []byte, counter, lamport, keyEpoch uint64, prevHash, nonce, ciphertext []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("event")
	if err := e.WriteUUID(eventID); err != nil {
		return nil, fmt.Errorf("event_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	if err := e.WriteDeviceID(deviceID); err != nil {
		return nil, fmt.Errorf("device_id: %w", err)
	}
	e.WriteU64(counter)
	e.WriteU64(lamport)
	e.WriteU64(keyEpoch)
	if err := e.WriteHash(prevHash); err != nil {
		return nil, fmt.Errorf("prev_hash: %w", err)
	}
	if err := e.WriteNonce(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	e.WriteBytes(ciphertext)
	return e.Bytes(), nil
}

func SignBytesMemberAdd(memberEventID, vaultID []byte, memberSeq uint64, prevHash, actorDeviceID, subjectDeviceID, inviteID, claimSig, subjectBundleSig, subjectPubkeySign, subjectPubkeyBox []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("member_add")
	if err := e.WriteUUID(memberEventID); err != nil {
		return nil, fmt.Errorf("member_event_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	e.WriteU64(memberSeq)
	if err := e.WriteHash(prevHash); err != nil {
		return nil, fmt.Errorf("prev_hash: %w", err)
	}
	if err := e.WriteDeviceID(actorDeviceID); err != nil {
		return nil, fmt.Errorf("actor_device_id: %w", err)
	}
	if err := e.WriteDeviceID(subjectDeviceID); err != nil {
		return nil, fmt.Errorf("subject_device_id: %w", err)
	}
	if err := e.WriteUUID(inviteID); err != nil {
		return nil, fmt.Errorf("invite_id: %w", err)
	}
	if err := e.WriteSignature(claimSig); err != nil {
		return nil, fmt.Errorf("claim_sig: %w", err)
	}
	if err := e.WriteSignature(subjectBundleSig); err != nil {
		return nil, fmt.Errorf("subject_bundle_sig: %w", err)
	}
	if err := e.WritePublicKey(subjectPubkeySign); err != nil {
		return nil, fmt.Errorf("subject_pubkey_sign: %w", err)
	}
	if err := e.WritePublicKey(subjectPubkeyBox); err != nil {
		return nil, fmt.Errorf("subject_pubkey_box: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesMemberRemove(memberEventID, vaultID []byte, memberSeq uint64, prevHash, actorDeviceID, subjectDeviceID []byte) ([]byte, error) {
	zero16 := make([]byte, 16)
	zero32 := make([]byte, 32)
	zero64 := make([]byte, 64)

	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("member_remove")
	if err := e.WriteUUID(memberEventID); err != nil {
		return nil, fmt.Errorf("member_event_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	e.WriteU64(memberSeq)
	if err := e.WriteHash(prevHash); err != nil {
		return nil, fmt.Errorf("prev_hash: %w", err)
	}
	if err := e.WriteDeviceID(actorDeviceID); err != nil {
		return nil, fmt.Errorf("actor_device_id: %w", err)
	}
	if err := e.WriteDeviceID(subjectDeviceID); err != nil {
		return nil, fmt.Errorf("subject_device_id: %w", err)
	}
	if err := e.WriteUUID(zero16); err != nil {
		return nil, fmt.Errorf("zero_uuid: %w", err)
	}
	if err := e.WriteSignature(zero64); err != nil {
		return nil, fmt.Errorf("zero_claim_sig: %w", err)
	}
	if err := e.WriteSignature(zero64); err != nil {
		return nil, fmt.Errorf("zero_bundle_sig: %w", err)
	}
	if err := e.WritePublicKey(zero32); err != nil {
		return nil, fmt.Errorf("zero_pubkey_sign: %w", err)
	}
	if err := e.WritePublicKey(zero32); err != nil {
		return nil, fmt.Errorf("zero_pubkey_box: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesInvite(inviteID, vaultID, targetDeviceID, targetPubkeySign, targetPubkeyBox, targetBundleSig, nonce, wrappedPayload, createdByDeviceID []byte, singleUse bool) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("invite")
	if err := e.WriteUUID(inviteID); err != nil {
		return nil, fmt.Errorf("invite_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	if err := e.WriteDeviceID(targetDeviceID); err != nil {
		return nil, fmt.Errorf("target_device_id: %w", err)
	}
	if err := e.WritePublicKey(targetPubkeySign); err != nil {
		return nil, fmt.Errorf("target_pubkey_sign: %w", err)
	}
	if err := e.WritePublicKey(targetPubkeyBox); err != nil {
		return nil, fmt.Errorf("target_pubkey_box: %w", err)
	}
	if err := e.WriteSignature(targetBundleSig); err != nil {
		return nil, fmt.Errorf("target_bundle_sig: %w", err)
	}
	if err := e.WriteNonce(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	e.WriteBytes(wrappedPayload)
	if err := e.WriteDeviceID(createdByDeviceID); err != nil {
		return nil, fmt.Errorf("created_by_device_id: %w", err)
	}
	e.WriteBool(singleUse)
	return e.Bytes(), nil
}

func SignBytesInviteClaim(inviteID, vaultID, deviceID []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("invite_claim")
	if err := e.WriteUUID(inviteID); err != nil {
		return nil, fmt.Errorf("invite_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	if err := e.WriteDeviceID(deviceID); err != nil {
		return nil, fmt.Errorf("device_id: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesKeyUpdate(keyUpdateID, vaultID []byte, memberSeq uint64, memberHeadHash, targetDeviceID []byte, keyEpoch uint64, nonce, wrappedPayload, createdByDeviceID []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("key_update")
	if err := e.WriteUUID(keyUpdateID); err != nil {
		return nil, fmt.Errorf("key_update_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	e.WriteU64(memberSeq)
	if err := e.WriteHash(memberHeadHash); err != nil {
		return nil, fmt.Errorf("member_head_hash: %w", err)
	}
	if err := e.WriteDeviceID(targetDeviceID); err != nil {
		return nil, fmt.Errorf("target_device_id: %w", err)
	}
	e.WriteU64(keyEpoch)
	if err := e.WriteNonce(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	e.WriteBytes(wrappedPayload)
	if err := e.WriteDeviceID(createdByDeviceID); err != nil {
		return nil, fmt.Errorf("created_by_device_id: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesKeyUpdateAck(vaultID, deviceID []byte, keyEpoch, memberSeq uint64, memberHeadHash []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("key_update_ack")
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	if err := e.WriteDeviceID(deviceID); err != nil {
		return nil, fmt.Errorf("device_id: %w", err)
	}
	e.WriteU64(keyEpoch)
	e.WriteU64(memberSeq)
	if err := e.WriteHash(memberHeadHash); err != nil {
		return nil, fmt.Errorf("member_head_hash: %w", err)
	}
	return e.Bytes(), nil
}

func SignBytesSnapshot(snapshotID, vaultID []byte, baseSeq, memberSeq uint64, memberHeadHash, baseCounterMap, headHashMap []byte, lamportAtSnapshot, keyEpoch uint64, nonce, ciphertext, createdByDeviceID []byte) ([]byte, error) {
	e := NewEncoder()
	e.WriteString(SignPrefix)
	e.WriteString("snapshot")
	if err := e.WriteUUID(snapshotID); err != nil {
		return nil, fmt.Errorf("snapshot_id: %w", err)
	}
	if err := e.WriteUUID(vaultID); err != nil {
		return nil, fmt.Errorf("vault_id: %w", err)
	}
	e.WriteU64(baseSeq)
	e.WriteU64(memberSeq)
	if err := e.WriteHash(memberHeadHash); err != nil {
		return nil, fmt.Errorf("member_head_hash: %w", err)
	}
	e.WriteBytes(baseCounterMap)
	e.WriteBytes(headHashMap)
	e.WriteU64(lamportAtSnapshot)
	e.WriteU64(keyEpoch)
	if err := e.WriteNonce(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	e.WriteBytes(ciphertext)
	if err := e.WriteDeviceID(createdByDeviceID); err != nil {
		return nil, fmt.Errorf("created_by_device_id: %w", err)
	}
	return e.Bytes(), nil
}
